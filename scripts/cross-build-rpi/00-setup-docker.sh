#!/usr/bin/env bash
# One-time: Docker CE + buildx + QEMU multi-arch emulation inside WSL2 Ubuntu.
# Only needed for 02-extract-sysroots.sh (pulling the lomorage buster images).
# Run manually in a WSL2 Ubuntu terminal -- needs your sudo password interactively.
set -ex

sudo apt-get update
sudo apt-get install -y ca-certificates curl gnupg

sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg

echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

sudo usermod -aG docker "$USER"
sudo systemctl enable --now docker

sudo docker run --rm --privileged tonistiigi/binfmt --install all

echo
echo "Setup done. Run 'wsl --shutdown' from PowerShell and reopen WSL so your"
echo "docker group membership takes effect."
