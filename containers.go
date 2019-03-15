package main

import (
	"fmt"
	docker "github.com/johnewart/go-dockerclient"
	"github.com/nu7hatch/gouuid"

	"os"
	"path/filepath"
	"strings"
)

type ServiceContext struct {
	DockerClient *docker.Client
	Id           string
	Containers   map[string]ContainerDefinition
	PackageName  string
	NetworkId    string
	Workspace    Workspace
}

type BuildContainerOpts struct {
	ContainerOpts ContainerDefinition
	PackageName   string
	Workspace     Workspace
}
type BuildContainer struct {
	Id      string
	Name    string
	Options BuildContainerOpts
}

func (sc *ServiceContext) FindContainer(cd ContainerDefinition) *docker.Container {
	return FindContainer(BuildContainerOpts{
		PackageName:   sc.PackageName,
		Workspace:     sc.Workspace,
		ContainerOpts: cd,
	})
}

// TODO: make sure the opts match the existing container
func FindContainer(opts BuildContainerOpts) *docker.Container {

	client := NewDockerClient()
	cd := opts.ContainerOpts

	s := strings.Split(cd.Image, ":")
	imageName := s[0]
	containerName := fmt.Sprintf("%s-%s", opts.PackageName, imageName)

	filters := make(map[string][]string)
	filters["name"] = append(filters["name"], containerName)

	result, err := client.ListContainers(docker.ListContainersOptions{
		Filters: filters,
		All:     true,
	})

	if err == nil && len(result) > 0 {
		for _, c := range result {
			fmt.Printf("ID: %s -- NAME: %s\n", c.ID, c.Names)
		}
		c := result[0]
		fmt.Printf("Found container %s with ID %s\n", containerName, c.ID)
		container, err := client.InspectContainer(c.ID)
		if err != nil {
			return nil
		} else {
			return container
		}
	} else {
		return nil
	}

}

func RemoveContainerById(id string) error {
	client := NewDockerClient()
	return client.RemoveContainer(docker.RemoveContainerOptions{
		ID: id,
	})
}

func (sc *ServiceContext) NewContainer(c ContainerDefinition) (BuildContainer, error) {
	opts := BuildContainerOpts{
		ContainerOpts: c,
		PackageName:   sc.PackageName,
		Workspace:     sc.Workspace,
	}
	return NewContainer(opts)
}

func NewDockerClient() *docker.Client {
	// TODO: Do something smarter...
	//endpoint := "unix:///var/run/docker.sock"
	endpoint := "unix:///home/jewart/.dockersocket"
	client, err := docker.NewVersionedClient(endpoint, "1.39")
	if err != nil {
		panic(err)
	}
	return client

}

func PullImage(imageLabel string) error {
	client := NewDockerClient()
	filters := make(map[string][]string)

	fmt.Printf("Pulling %s if needed...\n", imageLabel)
	filters["label"] = append(filters["label"], imageLabel)

	parts := strings.Split(imageLabel, ":")
	imageName := parts[0]
	imageTag := "latest"

	if len(parts) == 2 {
		imageTag = parts[1]
	}

	opts := docker.ListImagesOptions{
		Filters: filters,
	}

	imgs, err := client.ListImages(opts)
	if err != nil {
		fmt.Printf("Error getting image list: %v\n", err)
		return err
	}

	if len(imgs) == 0 {
		fmt.Printf("Image %s not found, pulling\n", imageLabel)

		pullOpts := docker.PullImageOptions{
			Repository:   imageName,
			Tag:          imageTag,
			OutputStream: os.Stdout,
		}

		authConfig := docker.AuthConfiguration{}

		if err = client.PullImage(pullOpts, authConfig); err != nil {
			fmt.Printf("Unable to pull image: %v\n", err)
			return err
		}

	}

	return nil

}

func (b BuildContainer) Start() error {
	client := NewDockerClient()

	hostConfig := &docker.HostConfig{}

	return client.StartContainer(b.Id, hostConfig)
}

func (b BuildContainer) ExecToStdout(cmdString string) error {
	client := NewDockerClient()

	fmt.Printf("Using API Version: %s\n", client.ServerAPIVersion())
	cmdArray := strings.Split(cmdString, " ")

	execOpts := docker.CreateExecOptions{
		Env:          b.Options.ContainerOpts.Environment,
		Cmd:          cmdArray,
		AttachStdout: true,
		AttachStderr: true,
		Container:    b.Id,
		WorkingDir:   "/build",
	}

	exec, err := client.CreateExec(execOpts)

	if err != nil {
		fmt.Printf("Can't create exec: %v\n", err)
		return err
	}

	startOpts := docker.StartExecOptions{
		OutputStream: os.Stdout,
		ErrorStream:  os.Stdout,
	}

	err = client.StartExec(exec.ID, startOpts)

	if err != nil {
		fmt.Printf("Unable to run exec %s: %v\n", exec.ID, err)
		return err
	}

	return nil

}

func NewContainer(opts BuildContainerOpts) (BuildContainer, error) {
	containerDef := opts.ContainerOpts
	s := strings.Split(containerDef.Image, ":")
	imageName := s[0]
	containerName := fmt.Sprintf("%s-%s", opts.PackageName, imageName)

	fmt.Printf("Creating container '%s'\n", containerName)

	client := NewDockerClient()

	PullImage(containerDef.Image)

	var mounts = make([]docker.HostMount, 0)

	buildRoot := opts.Workspace.BuildRoot()
	pkgWorkdir := filepath.Join(buildRoot, opts.PackageName)

	for _, mountSpec := range containerDef.Mounts {
		s := strings.Split(mountSpec, ":")
		src := filepath.Join(pkgWorkdir, s[0])

		MkdirAsNeeded(src)

		dst := s[1]

		mounts = append(mounts, docker.HostMount{
			Source: src,
			Target: dst,
			Type:   "bind",
		})
	}

	mounts = append(mounts, docker.HostMount{
		Source: opts.Workspace.PackagePath(opts.PackageName),
		Target: "/build",
		Type:   "bind",
	})

	var ports = make([]string, 0)
	for _, portSpec := range containerDef.Ports {
		ports = append(ports, portSpec)
	}

	cmdParts := strings.Split("tail -f /dev/null", " ")

	hostConfig := docker.HostConfig{
		Mounts: mounts,
	}

	config := docker.Config{
		AttachStdout: true,
		AttachStdin:  true,
		Image:        containerDef.Image,
		Cmd:          cmdParts,
		PortSpecs:    ports,
	}

	container, err := client.CreateContainer(
		docker.CreateContainerOptions{
			Name:       containerName,
			Config:     &config,
			HostConfig: &hostConfig,
		})

	if err != nil {
		return BuildContainer{}, err
	}

	fmt.Printf("Container ID: %s\n", container.ID)

	return BuildContainer{
		Name:    containerName,
		Id:      container.ID,
		Options: opts,
	}, nil
}

func NewServiceContext(packageName string, containers map[string]ContainerDefinition) (*ServiceContext, error) {
	workspace := LoadWorkspace()

	dockerClient := NewDockerClient()
	ctxId, _ := uuid.NewV4()
	/*log.Printf("Creating network %s...\n", ctxId.String())
	opts := types.NetworkCreate{}
	response, err := dockerClient.NetworkCreate(ctx, ctxId.String(), opts)
	if err != nil {
		return nil, err
	}*/

	sc := &ServiceContext{
		PackageName:  packageName,
		Id:           ctxId.String(),
		DockerClient: dockerClient,
		Containers:   containers,
		NetworkId:    "",
		Workspace:    workspace,
	}

	return sc, nil
}

func (sc *ServiceContext) SetupNetwork() (string, error) {
	/*ctx := context.Background()
	dockerClient := sc.DockerClient
	opts := types.NetworkCreate{}

	log.Printf("Creating network %s...\n", sc.Id)
	response, err := dockerClient.NetworkCreate(ctx, sc.Id, opts)

	if err != nil {
		return "", err
	}

	return response.ID, nil
	*/
	return "", nil
}

func (sc *ServiceContext) Cleanup() error {
	/*
		ctx := context.Background()
		dockerClient := sc.DockerClient

		log.Printf("Removing network %s\n", sc.Id)
		err := dockerClient.NetworkRemove(ctx, sc.Id)

		if err != nil {
			return err
		}
	*/
	return nil

}

func (sc *ServiceContext) StandUp() error {
	workspace := LoadWorkspace()
	buildRoot := workspace.BuildRoot()
	pkgWorkdir := filepath.Join(buildRoot, sc.PackageName)
	MkdirAsNeeded(pkgWorkdir)
	logDir := filepath.Join(pkgWorkdir, "logs")
	MkdirAsNeeded(logDir)

	for _, c := range sc.Containers {
		fmt.Printf("  %s...\n", c.Image)

		container := sc.FindContainer(c)

		if container != nil {
			continue
		} else {
			container, err := sc.NewContainer(c)
			if err != nil {
				return err
			}
			fmt.Printf("Created container: %s\n", container.Id)
		}

		/*log.Printf("Connecting to network %s...\n", sc.NetworkId)
		if err := dockerClient.NetworkConnect(ctx, sc.NetworkId, dependencyContainer.ID, &network.EndpointSettings{}); err != nil {
			panic(err)
		}

		log.Printf("Starting container...\n")
		if err := dockerClient.ContainerStart(ctx, dependencyContainer.ID, types.ContainerStartOptions{}); err != nil {
			panic(err)
		}

		props, err := dockerClient.ContainerInspect(ctx, dependencyContainer.ID)

		if err != nil {
			panic(err)
		}

		networkSettings := props.NetworkSettings
		ipv4 := networkSettings.DefaultNetworkSettings.IPAddress
		log.Printf("Container IP: %s\n", ipv4)

		//TODO: stream logs from each dependency to the build dir
		containerLogFile := filepath.Join(logDir, fmt.Sprintf("%s.log", imageName))
		f, err := os.Create(containerLogFile)

		if err != nil {
			fmt.Printf("Unable to write to log file %s: %v\n", containerLogFile, err)
			return err
		}

		out, err := dockerClient.ContainerLogs(ctx, dependencyContainer.ID, types.ContainerLogsOptions{
			ShowStderr: true,
			ShowStdout: true,
			Timestamps: false,
			Follow:     true,
			Tail:       "40",
		})
		if err != nil {
			fmt.Printf("Can't get log handle for container %s: %v\n", dependencyContainer.ID, err)
			return err
		}
		go func() {
			for {
				io.Copy(f, out)
				time.Sleep(300 * time.Millisecond)
			}
		}()
		*/
	}

	return nil
}
