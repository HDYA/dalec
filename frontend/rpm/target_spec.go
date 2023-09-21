package rpm

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"runtime/debug"

	"github.com/azure/dalec"
	"github.com/azure/dalec/frontend"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/image"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
)

const distroTargetKey = "TARGET_DISTRO"

func HandleSpec(ctx context.Context, client gwclient.Client, spec *dalec.Spec) (gwclient.Reference, *image.Image, error) {
	t, _ := frontend.GetBuildArg(client, distroTargetKey)
	st, err := Dalec2SpecLLB(spec, llb.Scratch(), t, "")
	if err != nil {
		return nil, nil, err
	}

	def, err := st.Marshal(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("error marshalling llb: %w", err)
	}

	res, err := client.Solve(ctx, gwclient.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return nil, nil, err
	}
	ref, err := res.SingleRef()
	// Do not return a nil image, it may cause a panic
	return ref, &image.Image{}, err
}

func isAnyDeps(spec *dalec.Spec) bool {
	for _, t := range spec.Targets {
		if len(t.Dependencies.Build) > 0 || len(t.Dependencies.Runtime) > 0 {
			return true
		}
	}
	return false
}

func Dalec2SpecLLB(spec *dalec.Spec, in llb.State, target, dir string) (llb.State, error) {
	buf := bytes.NewBuffer(nil)
	info, _ := debug.ReadBuildInfo()
	buf.WriteString("# Automatically generated by " + info.Main.Path + "\n")
	if target == "" && isAnyDeps(spec) {
		buf.WriteString("#\n")
		buf.WriteString("# WARNING: No distro target specified.\n")
		buf.WriteString("# If spec is invalid you may need to specify the target with the build arg " + distroTargetKey + "\n")
		buf.WriteString("#\n")
	}
	buf.WriteString("\n")

	if err := WriteSpec(spec, target, buf); err != nil {
		return llb.Scratch(), err
	}

	if dir == "" {
		dir = "SPECS/" + spec.Name
	}

	return in.
			File(llb.Mkdir(dir, 0755, llb.WithParents(true))).
			File(llb.Mkfile(filepath.Join(dir, spec.Name)+".spec", 0640, buf.Bytes())),
		nil
}
