package cmd

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

var (
	// remote or local source to get the tools from, must be a .tar.gz file
	toolSource string

	// if set to true, skip confirmation prompt and uninstall all tools
	skipUninstallConfirm bool

	toolCmd = &cobra.Command{
		Use:   "tool",
		Short: "Manage tools needed for performance testing",
		Long:  `Manage tools needed for performance testing.`,
	}

	toolInstallCmd = &cobra.Command{
		Use:   "install",
		Short: "Install tools needed for performance testing",
		Long:  `Install tools needed for performance testing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolsDir, err := toolsDirPath()
			if err != nil {
				return err
			}

			fmt.Println("installing benchmark tools...")
			fmt.Printf("* tools source: %s\n", toolSource)
			fmt.Printf("* target dir: %s\n", toolsDir)
			if err := os.MkdirAll(toolsDir, 0o755); err != nil {
				return fmt.Errorf("failed to create directory %s: %v", toolsDir, err)
			}

			// try to fetch the tools from the specified source, if it's a valid URL.
			// otherwise, try to read from local file
			var tarGz []byte
			if strings.HasPrefix(toolSource, "http://") || strings.HasPrefix(toolSource, "https://") {
				b, err := fetchFromURL(toolSource)
				if err != nil {
					return fmt.Errorf("failed to download tools from %s: %w", toolSource, err)
				}
				tarGz = b
			} else {
				b, err := os.ReadFile(toolSource)
				if err != nil {
					return fmt.Errorf("failed to read tools from local file %s: %w", toolSource, err)
				}
				tarGz = b
			}

			buf := bytes.NewBuffer(tarGz)
			if err := installTools(buf, toolsDir); err != nil {
				return err
			}
			return listTools()
		},
	}

	toolListCmd = &cobra.Command{
		Use:   "list",
		Short: "List tools needed for performance testing",
		Long:  `List tools needed for performance testing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listTools()
		},
	}

	toolUninstallCmd = &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall tools needed for performance testing",
		Long:  `Uninstall tools needed for performance testing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return uninstallTools()
		},
	}
)

func init() {
	rootCmd.AddCommand(toolCmd)
	toolCmd.AddCommand(toolInstallCmd)
	toolCmd.AddCommand(toolListCmd)
	toolCmd.AddCommand(toolUninstallCmd)
	toolCmd.SilenceUsage = true

	toolInstallCmd.Flags().StringVarP(&toolSource, "tool-source", "t", "", "Remote or local source to get the tools from. Must be a .tar.gz file")
	if err := toolInstallCmd.MarkFlagRequired("tool-source"); err != nil {
		klog.ErrorS(err, "failed to mark flag 'tool-source' as required")
	}
	toolInstallCmd.SilenceUsage = true

	toolListCmd.SilenceUsage = true

	toolUninstallCmd.Flags().BoolVarP(&skipUninstallConfirm, "skip-confirm", "y", false, "Skip confirmation prompt and uninstall all tools")
	toolUninstallCmd.SilenceUsage = true
}

func fetchFromURL(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download tools from %s: %v", toolSource, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download tools from %s: status code %d", toolSource, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func installTools(tarGz io.Reader, toolsDir string) error {
	// gunzip the .tar.gz file
	gr, err := gzip.NewReader(tarGz)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	//nolint:errcheck
	defer gr.Close()

	// access toolsDir as a root dir to prevent malicious file paths from escaping
	// the toolsDir
	root, err := os.OpenRoot(toolsDir)
	if err != nil {
		return fmt.Errorf("failed to open tools directory %s: %v", toolsDir, err)
	}
	// nolint:errcheck
	defer root.Close()

	// untar the .tar file
	tr := tar.NewReader(gr)
	var readErr error
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			readErr = errors.Join(readErr, fmt.Errorf("failed to read next tar entry: %w", err))
			break
		}

		if hdr.Typeflag != tar.TypeReg {
			continue // skip non-regular files
		}

		targetPath := filepath.Clean(hdr.Name)
		targetFile, err := root.OpenFile(targetPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, hdr.FileInfo().Mode())
		if err != nil {
			readErr = errors.Join(readErr, fmt.Errorf("failed to open target file %s: %v", targetPath, err))
			continue
		}

		if _, err := io.Copy(targetFile, tr); err != nil {
			readErr = errors.Join(readErr, fmt.Errorf("failed to copy to target file %s: %v", targetPath, err))
			if err := root.Remove(targetPath); err != nil { // remove the partially written file on error
				klog.V(3).ErrorS(err, "failed to remove partially written target file", "targetPath", targetPath)
			}
			continue
		}

		if err := targetFile.Close(); err != nil {
			readErr = errors.Join(readErr, fmt.Errorf("failed to close target file %s: %v", targetPath, err))
			continue
		}
	}
	return readErr
}

func listTools() error {
	toolsDir, err := toolsDirPath()
	if err != nil {
		return err
	}

	dir, err := os.ReadDir(toolsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("tools directory %q does not exist. Please run 'hvperf tool install' to install the tools first", toolsDir)
		}
		return err
	}

	fmt.Printf("listing benchmark tools (%s)...\n", toolsDir)
	for _, entry := range dir {
		fmt.Printf("  * %s\n", entry.Name())
	}
	return nil
}

func uninstallTools() error {
	toolsDir, err := toolsDirPath()
	if err != nil {
		return err
	}

	if skipUninstallConfirm || confirmUninstall(toolsDir) {
		return os.RemoveAll(toolsDir)
	}
	return nil
}

func confirmUninstall(toolsDir string) bool {
	r := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("Confirm uninstall all tools from %q? (y/n): ", toolsDir)
		input, err := r.ReadString('\n')
		if err != nil {
			klog.V(3).ErrorS(err, "failed to read user input")
			return false
		}

		if strings.EqualFold(strings.TrimSpace(input), "y") {
			return true
		} else if strings.EqualFold(strings.TrimSpace(input), "n") {
			return false
		}
		fmt.Println("Invalid input. Please enter 'y' or 'n'.")
	}
}

func toolsDirPath() (string, error) {
	d, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(d, ".hvperf", "tools"), nil
}
