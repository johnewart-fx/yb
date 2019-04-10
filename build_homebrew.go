package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/src-d/go-git.v4"
	"github.com/matishsiao/goInfo"
)

type HomebrewBuildTool struct {
	BuildTool
	_version string
}

func NewHomebrewBuildTool(toolSpec string) HomebrewBuildTool {
	parts := strings.Split(toolSpec, ":")
	version := parts[1]

	tool := HomebrewBuildTool{
		_version: version,
	}

	return tool
}

func (bt HomebrewBuildTool) Version() string {
	return bt._version
}

// Normally we want to put this in the tools dir; for now we put it in the build dir because I'm not 
// sure how to handle installation of multiple versions of things via Brew so this will allow project-specific 
// versioning
func (bt HomebrewBuildTool) HomebrewDir() string { 
	return filepath.Join(bt.InstallDir(), "brew")
}

func (bt HomebrewBuildTool) InstallDir() string { 
	workspace := LoadWorkspace()
	return filepath.Join(workspace.BuildRoot(), "homebrew")
}

func (bt HomebrewBuildTool) Install() error {
	gi := goInfo.GetInfo()
	if gi.GoOS != "darwin" { 
		return fmt.Errorf("Homebrew not supported on %s platform", gi.GoOS)
	}
	
	installDir := bt.InstallDir()
	brewDir := bt.HomebrewDir()

	MkdirAsNeeded(installDir)

	brewGitUrl := "https://github.com/Homebrew/brew.git"

	bt.InstallPlatformDependencies()

	if _, err := os.Stat(brewDir); err == nil {
		fmt.Printf("brew installed in %s\n", brewDir)
	} else {
		fmt.Printf("Installing brew\n")

		_, err := git.PlainClone(brewDir, false, &git.CloneOptions{
			URL:      brewGitUrl,
			Progress: os.Stdout,
		})

		if err != nil {
			fmt.Printf("Unable to clone brew!\n")
			return fmt.Errorf("Couldn't clone brew: %v\n", err)
		}
	}

	return nil
}

func (bt HomebrewBuildTool) Setup() error {
	brewDir := bt.HomebrewDir()
	brewBinDir := filepath.Join(brewDir, "bin")

	PrependToPath(brewBinDir)

	return nil

}

func (bt HomebrewBuildTool) InstallPlatformDependencies() error { 
	// Currently a no-op
	return nil
}
