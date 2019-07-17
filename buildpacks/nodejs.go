package buildpacks

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/johnewart/archiver"
	. "github.com/yourbase/yb/plumbing"
	. "github.com/yourbase/yb/types"
)

var NODE_DIST_MIRROR = "https://nodejs.org/dist"

type NodeBuildTool struct {
	BuildTool
	version string
	spec    BuildToolSpec
}

func NewNodeBuildTool(toolSpec BuildToolSpec) NodeBuildTool {
	tool := NodeBuildTool{
		version: toolSpec.Version,
		spec:    toolSpec,
	}

	return tool
}

func (bt NodeBuildTool) Version() string {
	return bt.version
}
func (bt NodeBuildTool) PackageString() string {
	version := bt.Version()
	arch := Arch()

	if arch == "amd64" {
		arch = "x64"
	}

	osName := OS()

	return fmt.Sprintf("node-v%s-%s-%s", version, osName, arch)
}

func (bt NodeBuildTool) NodeDir() string {
	return filepath.Join(bt.InstallDir(), bt.PackageString())
}

func (bt NodeBuildTool) InstallDir() string {
	return filepath.Join(bt.spec.SharedCacheDir, "nodejs")
}

func (bt NodeBuildTool) Install() error {

	nodeDir := bt.NodeDir()
	installDir := bt.InstallDir()
	nodePkgString := bt.PackageString()

	if _, err := os.Stat(nodeDir); err == nil {
		fmt.Printf("Node v%s located in %s!\n", bt.Version(), nodeDir)
	} else {
		fmt.Printf("Would install Node v%s into %s\n", bt.Version(), installDir)
		archiveFile := fmt.Sprintf("%s.tar.gz", nodePkgString)
		downloadUrl := fmt.Sprintf("%s/v%s/%s", NODE_DIST_MIRROR, bt.Version(), archiveFile)
		fmt.Printf("Downloading from URL %s...\n", downloadUrl)
		localFile, err := DownloadFileWithCache(downloadUrl)
		if err != nil {
			fmt.Printf("Unable to download: %v\n", err)
			return err
		}

		err = archiver.Unarchive(localFile, installDir)
		if err != nil {
			fmt.Printf("Unable to decompress: %v\n", err)
			return err
		}
	}

	return nil
}

func (bt NodeBuildTool) Setup() error {
	nodeDir := bt.NodeDir()
	cmdPath := filepath.Join(nodeDir, "bin")
	PrependToPath(cmdPath)
	// TODO: Fix this to be the package cache?
	nodePath := bt.spec.PackageDir
	fmt.Printf("Setting NODE_PATH to %s\n", nodePath)
	os.Setenv("NODE_PATH", nodePath)

	npmBinPath := filepath.Join(nodePath, "node_modules", ".bin")
	PrependToPath(npmBinPath)

	return nil
}
