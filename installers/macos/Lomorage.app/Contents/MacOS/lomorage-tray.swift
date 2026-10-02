// macOS menu bar app: a persistent icon in the menu bar with a click menu to open the web UI
// and start/stop/restart lomod, plus a destructive "reset to initial state" action. Mirrors
// installers/windows/lomorage-tray.ps1's system tray icon.
//
// This is a compiled Swift binary, not a JXA (JavaScript for Automation, `osascript -l
// JavaScript`) script -- an earlier version was JXA, matching this project's general
// "ships as plain interpreted source, no compiled binary" philosophy (see
// lomorage-tray.ps1/lomorage-start.sh/etc.). That had to be abandoned after finding a
// confirmed, reproducible, JXA-specific bug: the FIRST NSStatusItem a JXA-hosted process
// creates in a login session lands at a real on-screen position, but every subsequent one --
// e.g. after actually quitting and relaunching, or even just from a second `osascript`
// process -- silently gets placed off-screen at a fixed sentinel position and never renders,
// confirmed via the Accessibility API across dozens of controlled tests, and unrecoverable via
// any code-level fix (autosaveName, avoiding a second NSStatusItem entirely via hide-not-quit,
// explicit .visible = true, restarting SystemUIServer). A compiled Swift binary performing the
// exact same NSStatusItem creation, in the same login session, at the same point where JXA was
// already reliably broken, works perfectly on both first launch and every subsequent relaunch
// -- conclusively isolating the bug to JXA/OSAKit's bridge into AppKit, not WindowServer or
// this app's own logic.
//
// This binary doubles as both the Lomorage.app launcher (built as a universal arm64+x86_64
// Mach-O by scripts/macos/build-app-bundle.sh, matching the earlier lomorage-launcher.c stub
// it replaces -- see that file's own now-removed header comment for why a *shell script*
// launcher specifically breaks LaunchServices' native-architecture detection) and the tray
// logic itself, merged into one process instead of a launcher execing into a separate
// interpreter.
//
// Reads the actual lomod install dir from a stable external config file
// (~/Library/Application Support/Lomorage/tray-install-dir.txt, written by install.sh's
// install_app_bundle), never from anything inside this bundle -- mutating a signed .app after
// the fact invalidates its code signature seal ("a sealed resource is missing or invalid").
//
// Quit does NOT terminate this process -- it hides the status item and stops lomod, leaving
// the process running. Reopening (double-click/Spotlight/Launchpad) on an app LaunchServices
// already considers running does not spawn a new process -- it sends a standard "reopen" Apple
// Event to the existing one, surfaced via applicationShouldHandleReopen(_:hasVisibleWindows:)
// below. From the user's perspective this is indistinguishable from a real quit/relaunch (icon
// disappears, lomod stops, reopening brings it back instantly); the only difference is a
// dormant process lingers in the background using effectively no resources until the next
// reopen or logout.
//
// Unlike the Windows tray icon (NotifyIcon: left-click double-click opens the web UI,
// right-click shows the menu), a macOS NSStatusItem with an assigned menu shows that menu on
// any click -- there's no idiomatic single/double-click split here, so "Open Lomorage" is
// simply the first menu item instead.

import Cocoa

final class AppDelegate: NSObject, NSApplicationDelegate, NSMenuDelegate {
    var statusItem: NSStatusItem!
    var startItem: NSMenuItem!
    var stopItem: NSMenuItem!
    var versionItem: NSMenuItem!
    var statusLine: NSMenuItem!
    var installDir: String = ""

    func applicationDidFinishLaunching(_ notification: Notification) {
        guard let dir = readInstallDir(), FileManager.default.fileExists(atPath: dir + "/lomorage-start.sh") else {
            showAlert(
                title: "Lomorage",
                message: "Lomorage is not installed yet, or its install directory is incomplete. Run the installer first:\n\ncurl -fsSL https://lomorage.com/mac/install.sh | bash"
            )
            NSApp.terminate(nil)
            return
        }
        installDir = dir

        NSApp.setActivationPolicy(.accessory)

        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        statusItem.isVisible = true

        let iconPath = installDir + "/lomorage-tray.png"
        if let image = NSImage(contentsOfFile: iconPath) {
            image.size = NSSize(width: 18, height: 18)
            statusItem.button?.image = image
        } else {
            statusItem.button?.title = "Lomorage"
        }

        let menu = NSMenu()
        menu.delegate = self
        // Off, or AppKit re-enables every item that has a working action each time the menu
        // opens, overriding the isEnabled values refreshStatus sets on Start/Stop.
        menu.autoenablesItems = false

        func addItem(_ title: String, _ action: Selector) -> NSMenuItem {
            let item = NSMenuItem(title: title, action: action, keyEquivalent: "")
            item.target = self
            menu.addItem(item)
            return item
        }

        // No action: a label saying whether lomod is running, filled in by refreshStatus.
        statusLine = NSMenuItem(title: "", action: nil, keyEquivalent: "")
        statusLine.isEnabled = false
        menu.addItem(statusLine)
        menu.addItem(NSMenuItem.separator())
        _ = addItem("Open Lomorage", #selector(openClicked))
        menu.addItem(NSMenuItem.separator())
        startItem = addItem("Start", #selector(startClicked))
        stopItem = addItem("Stop", #selector(stopClicked))
        _ = addItem("Restart", #selector(restartClicked))
        menu.addItem(NSMenuItem.separator())
        _ = addItem("Reset to initial state…", #selector(resetClicked))
        menu.addItem(NSMenuItem.separator())
        // No action: just a greyed-out label, filled in by menuWillOpen.
        versionItem = NSMenuItem(title: "Version", action: nil, keyEquivalent: "")
        versionItem.isEnabled = false
        menu.addItem(versionItem)
        _ = addItem("Check for Updates…", #selector(checkForUpdatesClicked))
        menu.addItem(NSMenuItem.separator())
        _ = addItem("Quit", #selector(quitClicked))

        statusItem.menu = menu

        startLomodIfNeeded()

        // The menu bar icon is dimmed while lomod is stopped, so its state shows without
        // opening the menu. Polled: lomod can also die or be started outside the tray (a
        // crash, the updater, lomorage-stop.sh).
        refreshStatus()
        Timer.scheduledTimer(withTimeInterval: 5, repeats: true) { [weak self] _ in
            self?.refreshStatus()
        }
    }

    // Refresh right before the menu is shown, so it reflects whatever actually happened last
    // (e.g. lomod crashed, or was killed outside the tray) without waiting for the next poll.
    func menuWillOpen(_ menu: NSMenu) {
        refreshStatus()
        versionItem.title = "Version " + installedVersion()
    }

    func refreshStatus() {
        let running = isLomodRunning()
        startItem.isEnabled = !running
        stopItem.isEnabled = running
        statusLine.title = running ? "Lomorage is running" : "Lomorage is stopped"
        statusLine.image = NSImage(named: running ? NSImage.statusAvailableName : NSImage.statusNoneName)
        statusItem.button?.appearsDisabled = !running
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        if statusItem != nil && !statusItem.isVisible {
            statusItem.isVisible = true
            startLomodIfNeeded()
        }
        return false
    }

    // MARK: - Menu actions

    @objc func openClicked() {
        if let url = URL(string: "http://localhost:\(lomodPort())") {
            NSWorkspace.shared.open(url)
        }
    }

    @objc func startClicked() {
        startLomodIfNeeded()
        refreshStatusSoon()
    }

    @objc func stopClicked() {
        stopLomod()
        refreshStatusSoon()
    }

    @objc func restartClicked() {
        stopLomod()
        Thread.sleep(forTimeInterval: 0.5)
        startLomodIfNeeded()
        refreshStatusSoon()
    }

    // lomod is started in the background and takes a moment to exit on stop, so the state
    // right after a click isn't the one it settles into.
    func refreshStatusSoon() {
        DispatchQueue.main.asyncAfter(deadline: .now() + 1) { [weak self] in
            self?.refreshStatus()
        }
    }

    @objc func quitClicked() {
        stopLomod()
        statusItem.isVisible = false
        // Process stays alive from here -- see header comment. Reopening is handled by
        // applicationShouldHandleReopen(_:hasVisibleWindows:) above, not by anything running
        // again from this point.
    }

    // Wipes lomod's account/catalog state (assets.db, tokens, shares, admin.json) so it comes
    // back up on the first-run setup page, WITHOUT touching any actual photos: those live
    // under <BaseDir>/<username>/..., a sibling of var/etc, not inside either. Re-adding a
    // user won't show pre-existing files as already-synced until something rescans that folder
    // -- the bytes survive, the "already synced" bookkeeping in assets.db does not.
    @objc func resetClicked() {
        let alert = NSAlert()
        alert.messageText = "Reset Lomorage"
        alert.informativeText = "This will remove all accounts, tokens, and sharing/sync history, then restart lomod to the first-run setup page.\n\nYour photo files are NOT touched — they live in a separate folder from this state. Continue?"
        alert.alertStyle = .warning
        alert.addButton(withTitle: "Cancel")
        alert.addButton(withTitle: "Reset")
        guard alert.runModal() == .alertSecondButtonReturn else { return }

        guard let baseDir = lomodBaseDir() else {
            showAlert(title: "Reset failed", message: "--base not found in lomod.args")
            return
        }
        stopLomod()
        Thread.sleep(forTimeInterval: 0.5)
        runShell("rm -rf \(shQuote(baseDir + "/var")) \(shQuote(baseDir + "/etc"))")
        startLomodIfNeeded()
        notify(title: "Lomorage", text: "Reset complete. Open Lomorage to set it up again.")
    }

    // Doesn't run lomorage-update.sh itself: a successful update restarts this tray, and
    // launchd would take a script spawned from here down along with it, mid-update. Kickstarts
    // the daily update LaunchAgent (install.sh's register_autoupdate) instead, which launchd
    // runs as a job of its own; the request file makes that run report its outcome as a
    // notification (see lomorage-update.sh), since this process may not be around to.
    @objc func checkForUpdatesClicked() {
        let requestPath = (installDir as NSString).deletingLastPathComponent + "/update-notify-requested"
        FileManager.default.createFile(atPath: requestPath, contents: nil)
        let out = runShell("launchctl kickstart gui/$(id -u)/com.lomorage.lomod-update >/dev/null 2>&1 && echo ok")
        if out.trimmingCharacters(in: .whitespacesAndNewlines) != "ok" {
            try? FileManager.default.removeItem(atPath: requestPath)
            showAlert(
                title: "Lomorage",
                message: "Automatic updates are not set up for this install. Re-run the installer to update and enable them:\n\ncurl -fsSL https://lomorage.com/mac/install.sh | bash"
            )
        }
    }

    // MARK: - lomod process control

    func isLomodRunning() -> Bool {
        let out = runShell("pgrep -f \(shQuote(installDir + "/lomod")) >/dev/null 2>&1 && echo yes || echo no")
        return out.trimmingCharacters(in: .whitespacesAndNewlines) == "yes"
    }

    func startLomodIfNeeded() {
        if isLomodRunning() { return }
        runShell("nohup \(shQuote(installDir + "/lomorage-start.sh")) >/dev/null 2>&1 &")
    }

    func stopLomod() {
        runShell(shQuote(installDir + "/lomorage-stop.sh"))
    }

    func lomodArgsFileContents() -> String {
        runShell("cat \(shQuote(installDir + "/lomod.args")) 2>/dev/null")
    }

    func installedVersion() -> String {
        let version = (try? String(contentsOfFile: installDir + "/version.txt", encoding: .utf8)) ?? ""
        let trimmed = version.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? "unknown" : trimmed
    }

    func lomodPort() -> String {
        let args = lomodArgsFileContents()
        if let m = firstMatch(in: args, pattern: #"--port\s+(\d+)"#) {
            return m
        }
        return "8000"
    }

    func lomodBaseDir() -> String? {
        let args = lomodArgsFileContents()
        if let m = firstMatch(in: args, pattern: #"--base\s+"([^"]+)""#) {
            return m
        }
        if let m = firstMatch(in: args, pattern: #"--base\s+(\S+)"#) {
            return m
        }
        return nil
    }

    // MARK: - Small helpers

    func readInstallDir() -> String? {
        guard let home = ProcessInfo.processInfo.environment["HOME"] else { return nil }
        let configPath = home + "/Library/Application Support/Lomorage/tray-install-dir.txt"
        guard let contents = try? String(contentsOfFile: configPath, encoding: .utf8) else { return nil }
        let trimmed = contents.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }

    func shQuote(_ s: String) -> String {
        "'" + s.replacingOccurrences(of: "'", with: "'\\''") + "'"
    }

    @discardableResult
    func runShell(_ cmd: String) -> String {
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/bin/sh")
        task.arguments = ["-c", cmd]
        let outPipe = Pipe()
        task.standardOutput = outPipe
        task.standardError = Pipe()
        do {
            try task.run()
            task.waitUntilExit()
            let data = outPipe.fileHandleForReading.readDataToEndOfFile()
            return String(data: data, encoding: .utf8) ?? ""
        } catch {
            return ""
        }
    }

    // Returns the first regex capture group (or the whole match if the pattern has no group).
    func firstMatch(in text: String, pattern: String) -> String? {
        guard let regex = try? NSRegularExpression(pattern: pattern) else { return nil }
        let range = NSRange(text.startIndex..., in: text)
        guard let match = regex.firstMatch(in: text, range: range) else { return nil }
        let groupIndex = match.numberOfRanges > 1 ? 1 : 0
        guard let swiftRange = Range(match.range(at: groupIndex), in: text) else { return nil }
        return String(text[swiftRange])
    }

    func notify(title: String, text: String) {
        let notification = NSUserNotification()
        notification.title = title
        notification.informativeText = text
        NSUserNotificationCenter.default.deliver(notification)
    }

    func showAlert(title: String, message: String) {
        let alert = NSAlert()
        alert.messageText = title
        alert.informativeText = message
        alert.alertStyle = .critical
        alert.runModal()
    }
}

let delegate = AppDelegate()
NSApplication.shared.delegate = delegate
NSApplication.shared.run()
