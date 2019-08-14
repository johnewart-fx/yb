package buildpacks

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/yourbase/yb/plumbing"
	. "github.com/yourbase/yb/types"
)

type AnacondaBuildTool struct {
	BuildTool
	version         string
	spec            BuildToolSpec
	pyCompatibleNum int
}

var ANACONDA_DIST_MIRROR = "https://repo.continuum.io/miniconda/Miniconda{{.PyNum}}-{{.Version}}-{{.OS}}-{{.Arch}}.{{.Extension}}"

func NewAnaconda2BuildTool(toolSpec BuildToolSpec) AnacondaBuildTool {
	tool := AnacondaBuildTool{
		version:         toolSpec.Version,
		spec:            toolSpec,
		pyCompatibleNum: 2,
	}

	return tool
}

func NewAnaconda3BuildTool(toolSpec BuildToolSpec) AnacondaBuildTool {
	tool := AnacondaBuildTool{
		version:         toolSpec.Version,
		spec:            toolSpec,
		pyCompatibleNum: 3,
	}

	return tool
}

func (bt AnacondaBuildTool) Version() string {
	return bt.version
}

func (bt AnacondaBuildTool) InstallDir() string {
	return filepath.Join(bt.spec.PackageCacheDir, "miniconda", fmt.Sprintf("miniconda-%s", bt.Version()))
}

func (bt AnacondaBuildTool) Install() error {
	anacondaDir := bt.InstallDir()
	setupDir := bt.spec.PackageDir

	if _, err := os.Stat(anacondaDir); err == nil {
		fmt.Printf("anaconda installed in %s\n", anacondaDir)
	} else {
		fmt.Printf("Installing anaconda\n")

		downloadUrl := bt.DownloadUrl()

		fmt.Printf("Downloading Miniconda from URL %s...\n", downloadUrl)
		localFile, err := DownloadFileWithCache(downloadUrl)
		if err != nil {
			fmt.Printf("Unable to download: %v\n", err)
			return err
		}
		for _, cmd := range []string{
			fmt.Sprintf("chmod +x %s", localFile),
			fmt.Sprintf("bash %s -b -p %s", localFile, anacondaDir),
		} {
			fmt.Printf("Running: '%v' ", cmd)
			ExecToStdout(cmd, setupDir)
		}

	}

	return nil
}

func (bt AnacondaBuildTool) DownloadUrl() string {
	opsys := OS()
	arch := Arch()
	extension := "sh"
	version := bt.Version()

	if version == "" {
		version = "latest"
	}

	if arch == "amd64" {
		arch = "x86_64"
	}

	if opsys == "darwin" {
		opsys = "MacOSX"
	}

	if opsys == "linux" {
		opsys = "Linux"
	}

	if opsys == "windows" {
		opsys = "Windows"
		extension = "exe"
	}

	data := struct {
		PyNum     int
		OS        string
		Arch      string
		Version   string
		Extension string
	}{
		bt.pyCompatibleNum,
		opsys,
		arch,
		version,
		extension,
	}

	url, _ := TemplateToString(ANACONDA_DIST_MIRROR, data)

	return url
}

func (bt AnacondaBuildTool) Setup() error {
	installDir := bt.InstallDir()
	PrependToPath(filepath.Join(installDir, "bin"))
	setupDir := bt.spec.PackageDir

	for _, cmd := range []string{
		fmt.Sprintf("conda config --set always_yes yes --set changeps1 no"),
		fmt.Sprintf("conda update -q conda"),
	} {
		fmt.Printf("Running: '%v' ", cmd)
		ExecToStdout(cmd, setupDir)
	}

	return nil

}