' Launches lomod.exe with a fully hidden window (style 0) -- unlike `start /min`, which still
' creates a visible, minimized console window with its own taskbar entry, this creates no
' window at all. Invoked by lomorage-start.bat, which stays the stable entry point used by
' install.ps1, the Startup-folder autostart shortcut, and lomoupg's --postcmd hook.
Set fso = CreateObject("Scripting.FileSystemObject")
Set WshShell = CreateObject("WScript.Shell")
installDir = fso.GetParentFolderName(WScript.ScriptFullName)

Set argsFile = fso.OpenTextFile(installDir & "\lomod.args", 1)
lomodArgs = argsFile.ReadLine()
argsFile.Close

WshShell.CurrentDirectory = installDir
WshShell.Run """" & installDir & "\lomod.exe"" " & lomodArgs, 0, False
