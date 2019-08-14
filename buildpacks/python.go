package buildpacks

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/yourbase/yb/plumbing"
	. "github.com/yourbase/yb/types"
)

type PythonBuildTool struct {
	BuildTool
	version string
	spec    BuildToolSpec
}

var ANACONDA_URL_TEMPLATE = "https://repo.continuum.io/miniconda/Miniconda{{.PyNum}}-{{.Version}}-{{.OS}}-{{.Arch}}.{{.Extension}}"

const AnacondaToolVersion = "4.7.10"

func NewPythonBuildTool(toolSpec BuildToolSpec) PythonBuildTool {
	tool := PythonBuildTool{
		version: toolSpec.Version,
		spec:    toolSpec,
	}

	return tool
}

func (bt PythonBuildTool) Version() string {
	return bt.version
}

func (bt PythonBuildTool) AnacondaInstallDir() string {
	return filepath.Join(bt.spec.SharedCacheDir, "miniconda3", fmt.Sprintf("miniconda-%s", AnacondaToolVersion))
}

func (bt PythonBuildTool) EnvironmentDir() string {
	return filepath.Join(bt.spec.PackageCacheDir, "conda-python", bt.Version())
}

func (bt PythonBuildTool) Install() error {
	anacondaDir := bt.AnacondaInstallDir()
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

		// TODO: Windows
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

func (bt PythonBuildTool) DownloadUrl() string {
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
		3,
		opsys,
		arch,
		AnacondaToolVersion,
		extension,
	}

	url, _ := TemplateToString(ANACONDA_URL_TEMPLATE, data)

	return url
}

func (bt PythonBuildTool) Setup() error {
	condaDir := bt.AnacondaInstallDir()
	envDir := bt.EnvironmentDir()

	if _, err := os.Stat(envDir); err == nil {
		fmt.Printf("environment installed in %s\n", envDir)
	} else {
		currentPath := os.Getenv("PATH")
		newPath := fmt.Sprintf("PATH=%s:%s", filepath.Join(condaDir, "bin"), currentPath)
		setupDir := bt.spec.PackageDir
		condaBin := filepath.Join(condaDir, "bin", "conda")

		for _, cmd := range []string{
			fmt.Sprintf("%s config --set always_yes yes --set changeps1 no", condaBin),
			fmt.Sprintf("%s update -q conda", condaBin),
			fmt.Sprintf("%s create --prefix %s python=%s", condaBin, envDir, bt.Version()),
		} {
			fmt.Printf("Running: '%v' ", cmd)
			if err := ExecToStdoutWithEnv(cmd, setupDir, []string{newPath}); err != nil {
				fmt.Printf("Unable to run setup command: %s\n", cmd)
				return fmt.Errorf("Unable to run '%s': %v", cmd, err)
			}
		}
	}

	// Add new env to path
	PrependToPath(filepath.Join(envDir, "bin"))

	return nil

}