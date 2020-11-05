package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/google/shlex"
	"github.com/matishsiao/goInfo"
	"github.com/spf13/cobra"
	"github.com/yourbase/yb"
	"github.com/yourbase/yb/internal/biome"
	"github.com/yourbase/yb/internal/build"
	"github.com/yourbase/yb/internal/plumbing"
	"github.com/yourbase/yb/internal/ybdata"
	"github.com/yourbase/yb/internal/ybtrace"
	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/codes"
	exporttrace "go.opentelemetry.io/otel/sdk/export/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"zombiezen.com/go/log"
)

const TIME_FORMAT = "15:04:05 MST"

type buildCmd struct {
	execPrefix       string
	noContainer      bool
	dependenciesOnly bool
}

func newBuildCmd() *cobra.Command {
	b := new(buildCmd)
	c := &cobra.Command{
		Use:   "build [options] [TARGET]",
		Short: "Build a target",
		Long: `Builds a target in the current package. If no argument is given, ` +
			`uses the target named "default", if there is one.`,
		Args:                  cobra.MaximumNArgs(1),
		DisableFlagsInUseLine: true,
		SilenceErrors:         true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "default"
			if len(args) > 0 {
				target = args[0]
			}
			return b.run(cmd.Context(), target)
		},
	}
	c.Flags().BoolVar(&b.noContainer, "no-container", false, "Avoid using Docker if possible")
	c.Flags().BoolVar(&b.dependenciesOnly, "deps-only", false, "Install only dependencies, don't do anything else")
	c.Flags().StringVar(&b.execPrefix, "exec-prefix", "", "Add a prefix to all executed commands (useful for timing or wrapping things)")
	return c
}

func (b *buildCmd) run(ctx context.Context, buildTargetName string) error {
	// Set up trace sink.
	buildTraces := new(traceSink)
	tp, err := sdktrace.NewProvider(sdktrace.WithSyncer(buildTraces))
	if err != nil {
		return err
	}
	global.SetTraceProvider(tp)

	// Obtain global dependencies.
	dataDirs, err := ybdata.DirsFromEnv()
	if err != nil {
		return err
	}
	execPrefix, err := shlex.Split(b.execPrefix)
	if err != nil {
		return fmt.Errorf("parse --exec-prefix: %w", err)
	}
	dockerClient, err := connectDockerClient(!b.noContainer)
	if err != nil {
		return err
	}

	startTime := time.Now()
	ctx, span := ybtrace.Start(ctx, "Build", trace.WithNewRoot())
	defer span.End()

	if plumbing.InsideTheMatrix() {
		startSection("BUILD CONTAINER")
	} else {
		startSection("BUILD HOST")
	}
	gi := goInfo.GetInfo()
	gi.VarDump()

	startSection("BUILD PACKAGE SETUP")
	log.Infof(ctx, "Build started at %s", startTime.Format(TIME_FORMAT))

	targetPackage, err := GetTargetPackage()
	if err != nil {
		return err
	}

	// Determine targets to build.
	buildTargets, err := targetPackage.Manifest.BuildOrder(buildTargetName)
	if err != nil {
		return fmt.Errorf("%w\nValid build targets: %s", err, strings.Join(targetPackage.Manifest.BuildTargetList(), ", "))
	}

	// Do the build!
	startSection("BUILD")
	log.Debugf(ctx, "Building package %s in %s...", targetPackage.Name, targetPackage.Path)
	log.Debugf(ctx, "Checksum of dependencies: %s", targetPackage.Manifest.BuildDependenciesChecksum())

	buildError := doTargetList(ctx, targetPackage, buildTargets, &doOptions{
		dockerClient: dockerClient,
		dataDirs:     dataDirs,
		execPrefix:   execPrefix,
		setupOnly:    b.dependenciesOnly,
	})
	if buildError != nil {
		span.SetStatus(codes.Unknown, buildError.Error())
	}
	span.End()
	endTime := time.Now()
	buildTime := endTime.Sub(startTime)

	log.Infof(ctx, "")
	log.Infof(ctx, "Build finished at %s, taking %s", endTime.Format(TIME_FORMAT), buildTime)
	log.Infof(ctx, "")

	log.Infof(ctx, "%s", buildTraces.dump())

	if buildError != nil {
		subSection("BUILD FAILED")
		return err
	}

	subSection("BUILD SUCCEEDED")
	return nil
}

type doOptions struct {
	dataDirs        *ybdata.Dirs
	dockerClient    *docker.Client
	dockerNetworkID string
	execPrefix      []string
	setupOnly       bool
}

func doTargetList(ctx context.Context, pkg *yb.Package, targets []*yb.BuildTarget, opts *doOptions) error {
	if len(targets) == 0 {
		return nil
	}
	orderMsg := new(strings.Builder)
	orderMsg.WriteString("Going to build targets in the following order:")
	for _, target := range targets {
		fmt.Fprintf(orderMsg, "\n   - %s", target.Name)
	}
	log.Debugf(ctx, "%s", orderMsg)

	// Create a Docker network, if needed.
	if opts.dockerClient != nil && opts.dockerNetworkID == "" {
		opts2 := new(doOptions)
		*opts2 = *opts
		var cleanup func()
		var err error
		opts2.dockerNetworkID, cleanup, err = newDockerNetwork(ctx, opts.dockerClient)
		if err != nil {
			return err
		}
		defer cleanup()
		opts = opts2
	}
	for _, target := range targets {
		err := doTarget(ctx, pkg, target, opts)
		if err != nil {
			return err
		}
	}
	return nil
}

func doTarget(ctx context.Context, pkg *yb.Package, target *yb.BuildTarget, opts *doOptions) error {
	bio, err := newBiome(ctx, opts.dockerClient, opts.dataDirs, pkg.Path, target.Name)
	if err != nil {
		return fmt.Errorf("target %s: %w", target.Name, err)
	}
	defer func() {
		if err := bio.Close(); err != nil {
			log.Warnf(ctx, "Clean up environment: %v", err)
		}
	}()
	sys := build.Sys{
		Biome:           bio,
		DataDirs:        opts.dataDirs,
		HTTPClient:      http.DefaultClient,
		DockerClient:    opts.dockerClient,
		DockerNetworkID: opts.dockerNetworkID,
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
	}
	phaseDeps, err := targetToPhaseDeps(target)
	if err != nil {
		return err
	}
	execBiome, err := build.Setup(ctx, sys, phaseDeps)
	if err != nil {
		return err
	}
	sys.Biome = biome.ExecPrefix{
		Biome:       execBiome,
		PrependArgv: opts.execPrefix,
	}
	defer func() {
		if err := execBiome.Close(); err != nil {
			log.Errorf(ctx, "Clean up target %s: %v", target.Name, err)
		}
	}()
	if opts.setupOnly {
		return nil
	}

	subSection(fmt.Sprintf("Build target: %s", target.Name))
	log.Infof(ctx, "Executing build steps...")
	err = build.Execute(ctx, sys, targetToPhase(target))
	if err != nil {
		return err
	}
	return nil
}

// A traceSink records spans in memory. The zero value is an empty sink.
type traceSink struct {
	mu        sync.Mutex
	rootSpans []*exporttrace.SpanData
	children  map[trace.SpanID][]*exporttrace.SpanData
}

// ExportSpan saves the trace span. It is safe to be called concurrently.
func (sink *traceSink) ExportSpan(_ context.Context, span *exporttrace.SpanData) {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if !span.ParentSpanID.IsValid() {
		sink.rootSpans = append(sink.rootSpans, span)
		return
	}
	if sink.children == nil {
		sink.children = make(map[trace.SpanID][]*exporttrace.SpanData)
	}
	sink.children[span.ParentSpanID] = append(sink.children[span.ParentSpanID], span)
}

const (
	traceDumpStartWidth   = 14
	traceDumpEndWidth     = 14
	traceDumpElapsedWidth = 14
)

// dump formats the recorded traces as a hierarchial table of spans in the order
// received. It is safe to call concurrently, including with ExportSpan.
func (sink *traceSink) dump() string {
	sb := new(strings.Builder)
	fmt.Fprintf(sb, "%-*s %-*s %-*s\n",
		traceDumpStartWidth, "Start",
		traceDumpEndWidth, "End",
		traceDumpElapsedWidth, "Elapsed",
	)
	sink.mu.Lock()
	sink.dumpLocked(sb, trace.SpanID{}, 0)
	sink.mu.Unlock()
	return sb.String()
}

func (sink *traceSink) dumpLocked(sb *strings.Builder, parent trace.SpanID, depth int) {
	const indent = "  "
	list := sink.rootSpans
	if parent.IsValid() {
		list = sink.children[parent]
	}
	if depth >= 3 {
		if len(list) > 0 {
			writeSpaces(sb, traceDumpStartWidth+traceDumpEndWidth+traceDumpElapsedWidth+3)
			for i := 0; i < depth; i++ {
				sb.WriteString(indent)
			}
			sb.WriteString("...\n")
		}
		return
	}
	for _, span := range list {
		elapsed := span.EndTime.Sub(span.StartTime)
		fmt.Fprintf(sb, "%-*s %-*s %*.3fs %s\n",
			traceDumpStartWidth, span.StartTime.Format(TIME_FORMAT),
			traceDumpEndWidth, span.EndTime.Format(TIME_FORMAT),
			traceDumpElapsedWidth-1, elapsed.Seconds(),
			strings.Repeat(indent, depth)+span.Name,
		)
		sink.dumpLocked(sb, span.SpanContext.SpanID, depth+1)
	}
}

func startSection(name string) {
	fmt.Printf(" === %s ===\n", name)
}

func subSection(name string) {
	fmt.Printf(" -- %s -- \n", name)
}

func writeSpaces(w io.ByteWriter, n int) {
	for i := 0; i < n; i++ {
		w.WriteByte(' ')
	}
}