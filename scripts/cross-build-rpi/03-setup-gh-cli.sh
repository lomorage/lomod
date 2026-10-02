#!/usr/bin/env bash
# One-time: installs GitHub CLI (gh) in WSL2 Ubuntu for publishing releases.
# Run manually in a WSL2 Ubuntu terminal -- needs your sudo password interactively.
set -ex

sudo mkdir -p -m 755 /etc/apt/keyrings
curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo tee /etc/apt/keyrings/githubcli-archive-keyring.gpg > /dev/null
sudo chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | \
  sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null

sudo apt-get update
sudo apt-get install -y gh

gh --version
echo
echo "Now run: gh auth login"
echo "(pick GitHub.com, HTTPS, and log in via browser or a personal access token)"
