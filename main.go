package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type Config struct {
	RepoURL        string `json:"repo_url"`
	Branch         string `json:"branch"`
	RepoPath       string `json:"repo_path"`
	DiscordWebhook string `json:"discord_webhook"`
	LastCommitHash string `json:"last_commit_hash"`
}

func loadConfig() (*Config, error) {
	file, err := os.Open("config.json")
	if err != nil {
		return nil, fmt.Errorf("failed to open config.json: %v", err)
	}
	defer file.Close()

	var cfg Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config.json: %v", err)
	}
	return &cfg, nil
}

func saveConfig(cfg *Config) error {
	file, err := os.Create("config.json")
	if err != nil {
		return fmt.Errorf("failed to create config.json: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("failed to encode config.json: %v", err)
	}
	return nil
}

func getRemoteHash(repoPath, branch string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "fetch", "origin", branch)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git fetch failed: %v", err)
	}

	cmd = exec.Command("git", "-C", repoPath, "rev-parse", fmt.Sprintf("origin/%s", branch))
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getLocalHash(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func isRepoClean(repoPath string) (bool, error) {
	cmd := exec.Command("git", "-C", repoPath, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "", nil
}

func pullRepo(repoPath, branch string) error {
	cmds := [][]string{
		{"git", "-C", repoPath, "fetch", "origin", branch},
		{"git", "-C", repoPath, "reset", "--hard", fmt.Sprintf("origin/%s", branch)},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git command failed: %v", err)
		}
	}
	return nil
}

func rebuildNixOS() error {
	cmd := exec.Command("nixos-rebuild", "switch")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func sendDiscord(webhookURL, message string) {
	payload := map[string]string{"content": message}
	data, _ := json.Marshal(payload)
	_, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("failed to send Discord webhook: %v", err)
	}
}

func isGitRepo(path string) bool {
	gitDir := fmt.Sprintf("%s/.git", path)
	info, err := os.Stat(gitDir)
	return err == nil && info.IsDir()
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
		log.Printf("failed to get hostname: %v", err)
	}

	if !isGitRepo(cfg.RepoPath) {
		sendDiscord(cfg.DiscordWebhook, fmt.Sprintf("Repo path `%s` on `%s` is not a Git repo", cfg.RepoPath, hostname))
		log.Fatalf("Not a Git repo: %s", cfg.RepoPath)
	}

	isClean, err := isRepoClean(cfg.RepoPath)
	if err != nil {
		sendDiscord(cfg.DiscordWebhook, fmt.Sprintf("Failed to check Git status on `%s`: %v", hostname, err))
		return
	}
	if !isClean {
		sendDiscord(cfg.DiscordWebhook, fmt.Sprintf("Repo at `%s` on `%s` has uncommitted changes. Skipping rebuild.", cfg.RepoPath, hostname))
		return
	}

	remoteHash, err := getRemoteHash(cfg.RepoPath, cfg.Branch)
	if err != nil {
		sendDiscord(cfg.DiscordWebhook, fmt.Sprintf("Failed to get remote hash on `%s`: %v", hostname, err))
		return
	}

	localHash, err := getLocalHash(cfg.RepoPath)
	if err != nil {
		sendDiscord(cfg.DiscordWebhook, fmt.Sprintf("Failed to get local hash on `%s`: %v", hostname, err))
		return
	}

	log.Printf("Remote Hash: %s", remoteHash)
	log.Printf("Local Hash: %s", localHash)

	if remoteHash == localHash {
		log.Printf("No changes detected.")
		return
	}

	log.Printf("Change detected! Pulling and rebuilding...")

	if err := pullRepo(cfg.RepoPath, cfg.Branch); err != nil {
		sendDiscord(cfg.DiscordWebhook, fmt.Sprintf("Failed to pull repo on `%s`: %v", hostname, err))
		return
	}

	if err := rebuildNixOS(); err != nil {
		sendDiscord(cfg.DiscordWebhook, fmt.Sprintf("NixOS rebuild failed on `%s`: %v", hostname, err))
		return
	}

	cfg.LastCommitHash = remoteHash
	if err := saveConfig(cfg); err != nil {
		log.Printf("Failed to save config: %v", err)
	}

	log.Printf("NixOS rebuild successful.")
	sendDiscord(cfg.DiscordWebhook, fmt.Sprintf("NixOS rebuild successful on `%s`.", hostname))
}
