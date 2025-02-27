package rpm

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/dalec"
	"github.com/Azure/dalec/frontend"
	"github.com/moby/buildkit/client/llb"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

func HandleSources(wf WorkerFunc) gwclient.BuildFunc {
	return func(ctx context.Context, client gwclient.Client) (*gwclient.Result, error) {
		return frontend.BuildWithPlatform(ctx, client, func(ctx context.Context, client gwclient.Client, platform *ocispecs.Platform, spec *dalec.Spec, targetKey string) (gwclient.Reference, *dalec.DockerImageSpec, error) {
			sOpt, err := frontend.SourceOptFromClient(ctx, client)
			if err != nil {
				return nil, nil, err
			}

			worker, err := wf(sOpt.Resolver, spec, targetKey)
			if err != nil {
				return nil, nil, err
			}

			sources, err := Dalec2SourcesLLB(worker, spec, sOpt)
			if err != nil {
				return nil, nil, err
			}

			// Now we can merge sources into the desired path
			st := dalec.MergeAtPath(llb.Scratch(), sources, "/SOURCES")

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
			if err != nil {
				return nil, nil, err
			}
			return ref, &dalec.DockerImageSpec{}, nil
		})
	}
}

func buildScriptSourceState(spec *dalec.Spec) *llb.State {
	if len(spec.Build.Steps) == 0 {
		return nil
	}

	script := buildScript(spec)
	st := llb.Scratch().File(llb.Mkfile("build.sh", 0755, []byte(script)))
	return &st
}

func buildScript(spec *dalec.Spec) string {
	b := &strings.Builder{}

	t := spec.Build
	if len(t.Steps) == 0 {
		return ""
	}

	fmt.Fprintln(b, "#!/bin/sh")
	fmt.Fprintln(b, "set -e")

	if spec.HasGomods() {
		// Older go versions did not have support for the `GOMODCACHE` var
		// This is a hack to try and make the build work by linking the go modules
		// we've already fetched into to module dir under $GOPATH
		// The default GOMODCACHE value is ${GOPATH}/pkg/mod.
		fmt.Fprintf(b, `test -n "$(go env GOMODCACHE)" || (GOPATH="$(go env GOPATH)"; mkdir -p "${GOPATH}/pkg" && ln -s "$(pwd)/%s" "${GOPATH}/pkg/mod")`, gomodsName)
		// Above command does not have a newline due to quoting issues, so add that here.
		fmt.Fprint(b, "\n")

		fmt.Fprintln(b, "export GOMODCACHE=\"$(pwd)/"+gomodsName+"\"")
	}

	envKeys := dalec.SortMapKeys(t.Env)
	for _, k := range envKeys {
		v := t.Env[k]
		fmt.Fprintf(b, "export %s=\"%s\"\n", k, v)
	}

	for _, step := range t.Steps {
		writeStep(b, step)
	}

	b.WriteString("\n")
	return b.String()
}

func Dalec2SourcesLLB(worker llb.State, spec *dalec.Spec, sOpt dalec.SourceOpts, opts ...llb.ConstraintsOpt) ([]llb.State, error) {
	sources, err := dalec.Sources(spec, sOpt)
	if err != nil {
		return nil, err
	}
	out := make([]llb.State, 0, len(sources))

	withPG := func(s string) []llb.ConstraintsOpt {
		return append(opts, dalec.ProgressGroup(s))
	}

	st, err := spec.GomodDeps(sOpt, worker, withPG("Add gomod sources")...)
	if err != nil {
		return nil, errors.Wrap(err, "error adding gomod sources")
	}

	sorted := dalec.SortMapKeys(sources)
	for _, k := range sorted {
		st := sources[k]
		if dalec.SourceIsDir(spec.Sources[k]) {
			st = st.With(sourceTar(worker, k, withPG("Tar source: "+k)...))
		}
		out = append(out, st)
	}

	if st != nil {
		out = append(out, st.With(sourceTar(worker, gomodsName, withPG("Tar gomod deps")...)))
	}

	scriptSt := buildScriptSourceState(spec)
	if scriptSt != nil {
		out = append(out, *scriptSt)
	}

	return out, nil
}

func sourceTar(worker llb.State, key string, opts ...llb.ConstraintsOpt) llb.StateOption {
	return func(in llb.State) llb.State {
		return dalec.Tar(worker, in, key+".tar.gz", opts...)
	}
}
