# nix-server-conf-manager
A simple nixos config manager written in golang

# How to use
Configure from which Repository to pull by editing the config.json file.
The program will automatically check if the local Repository matches the latest version. If not it pulls the newest version and rebuilds nixos.

Error Messages are reported using Discord Webhooks. See this Guide to webhooks: https://support.discord.com/hc/en-us/articles/228383668-Intro-to-Webhooks 