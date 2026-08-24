package main

import (
	_ "embed"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/jlaffaye/ftp"
	"gopkg.in/yaml.v3"
)

//go:embed config.yaml
var embeddedConfig []byte

// Config represents the configuration file structure
type Config struct {
	Translations []TranslationConfig `yaml:"translations"`
}

// TranslationConfig represents a single translation configuration
type TranslationConfig struct {
	Name        string    `yaml:"name"`
	FullName    string    `yaml:"fullName"`
	Description string    `yaml:"description"`
	Testaments  string    `yaml:"testaments"` // "both", "ot", "nt" - defaults to "both" if not specified
	FTP         FTPConfig `yaml:"ftp"`
}

// FTPConfig represents FTP connection details
type FTPConfig struct {
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	User      string `yaml:"user"`
	Password  string `yaml:"password"`
	Directory string `yaml:"directory"`
}

func translationsDir() string {
	if dir := os.Getenv("TRANSLATIONS_DIR"); dir != "" {
		return dir
	}

	return filepath.Join(os.TempDir(), "bible-soh-re", "translations")
}

// LoadConfig loads the configuration from a file or falls back to embedded config.yaml
func LoadConfig(filename string) (*Config, error) {
	var data []byte
	var err error

	// Try to read from file first
	data, err = os.ReadFile(filename)
	if err != nil {
		// Fall back to embedded config
		log.Printf("Config file %s not found, using embedded config", filename)
		data = embeddedConfig
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// DownloadTranslation downloads all files from FTP for a translation
func DownloadTranslation(tc TranslationConfig) error {
	log.Printf("Downloading translation: %s from %s:%d", tc.Name, tc.FTP.Host, tc.FTP.Port)

	// Create local directory
	localDir := filepath.Join(translationsDir(), tc.Name)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", localDir, err)
	}

	// Connect to FTP server
	addr := fmt.Sprintf("%s:%d", tc.FTP.Host, tc.FTP.Port)
	conn, err := ftp.Dial(addr)
	if err != nil {
		return fmt.Errorf("failed to connect to FTP server: %w", err)
	}
	defer func() {
		if err := conn.Quit(); err != nil {
			log.Printf("Failed to quit FTP connection: %v", err)
		}
	}()

	// Login
	if err := conn.Login(tc.FTP.User, tc.FTP.Password); err != nil {
		return fmt.Errorf("failed to login to FTP server: %w", err)
	}

	// Change to the specified directory
	if err := conn.ChangeDir(tc.FTP.Directory); err != nil {
		return fmt.Errorf("failed to change to directory %s: %w", tc.FTP.Directory, err)
	}

	// List all files in the directory
	entries, err := conn.List(".")
	if err != nil {
		return fmt.Errorf("failed to list directory: %w", err)
	}

	// Download each file
	for _, entry := range entries {
		if entry.Type == ftp.EntryTypeFile {
			remotePath := entry.Name
			localPath := filepath.Join(localDir, entry.Name)

			log.Printf("Downloading %s to %s", remotePath, localPath)

			// Retrieve file
			resp, err := conn.Retr(remotePath)
			if err != nil {
				log.Printf("Failed to retrieve %s: %v", remotePath, err)
				continue
			}

			// Create local file
			outFile, err := os.Create(localPath)
			if err != nil {
				if closeErr := resp.Close(); closeErr != nil {
					log.Printf("Failed to close response: %v", closeErr)
				}
				log.Printf("Failed to create local file %s: %v", localPath, err)
				continue
			}

			// Copy content
			_, err = io.Copy(outFile, resp)
			if closeErr := outFile.Close(); closeErr != nil {
				log.Printf("Failed to close file %s: %v", localPath, closeErr)
			}
			if closeErr := resp.Close(); closeErr != nil {
				log.Printf("Failed to close response: %v", closeErr)
			}

			if err != nil {
				log.Printf("Failed to download %s: %v", remotePath, err)
				continue
			}

			log.Printf("Successfully downloaded %s", remotePath)
		}
	}

	log.Printf("Finished downloading translation: %s", tc.Name)
	return nil
}

// DownloadAllTranslations downloads all configured translations
func DownloadAllTranslations(config *Config) error {
	for _, tc := range config.Translations {
		if err := DownloadTranslation(tc); err != nil {
			log.Printf("Error downloading %s: %v", tc.Name, err)
			// Continue with next translation instead of failing completely
			continue
		}
	}
	return nil
}

// TranslationExists checks if a translation directory exists and has required files
func TranslationExists(name string) bool {
	localDir := filepath.Join(translationsDir(), name)

	// Check if directory exists
	info, err := os.Stat(localDir)
	if err != nil || !info.IsDir() {
		return false
	}

	// Check for required files (ot.bzv, ot.bzs, ot.bzz, nt.bzv, nt.bzs, nt.bzz)
	requiredFiles := []string{
		filepath.Join(localDir, "ot.bzv"),
		filepath.Join(localDir, "ot.bzs"),
		filepath.Join(localDir, "ot.bzz"),
		filepath.Join(localDir, "nt.bzv"),
		filepath.Join(localDir, "nt.bzs"),
		filepath.Join(localDir, "nt.bzz"),
	}

	for _, file := range requiredFiles {
		if _, err := os.Stat(file); err != nil {
			return false
		}
	}

	return true
}

// EnsureTranslationsExist checks if translations exist, downloads them if missing
func EnsureTranslationsExist(config *Config) error {
	for _, tc := range config.Translations {
		if !TranslationExists(tc.Name) {
			log.Printf("Translation %s not found locally, downloading...", tc.Name)
			if err := DownloadTranslation(tc); err != nil {
				log.Printf("Error downloading %s: %v", tc.Name, err)
				// Continue with next translation instead of failing
				continue
			}
		} else {
			log.Printf("Translation %s already exists locally", tc.Name)
		}
	}

	return nil
}
