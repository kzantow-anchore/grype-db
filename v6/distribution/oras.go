package distribution

import (
	"bytes"
	"context"
	"fmt"
	"github.com/anchore/grype-db/internal/log"
	"io"
	"io/fs"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras/cmd/oras/root"
	"os"
	"path/filepath"
	"strings"
)

type OrasSource struct {
	PlainHTTP bool
}

func (o OrasSource) Handles(url string) bool {
	return strings.HasPrefix(url, "oras:")
}

func (o OrasSource) Push(ctx context.Context, url string, mediaType string, contentReader io.Reader) (err error) {
	url = trimScheme(url, "oras")
	return o.pushEmbeddedCommand(ctx, url, mediaType, contentReader)
}

func (o OrasSource) pushEmbeddedCommand(ctx context.Context, reference string, mediaType string, contentReader io.Reader) (err error) {
	cmds := root.New()

	filePath := ""

	if f, ok := contentReader.(fs.File); ok {
		fi, err := f.Stat()
		if err != nil {
			return err
		}
		filePath, err = filepath.Abs(fi.Name())
		if err != nil {
			return err
		}
	} else {
		tmpDir, err := os.MkdirTemp("", "pull-contents-")
		if err != nil {
			return err
		}
		defer func() {
			_ = os.RemoveAll(tmpDir)
		}()

		tmpFilePath, err := filepath.Abs(filepath.Join(tmpDir, "tmp"))
		if err != nil {
			return err
		}
		tmpFile, err := os.OpenFile(tmpFilePath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(tmpFile, contentReader)
		closeOrLog(tmpFilePath, tmpFile)
		if err != nil {
			return err
		}
		filePath = tmpFilePath
	}

	// push localhost:5000/hello:v1 hi.txt:application/vnd.me.hi
	cmds.SetArgs([]string{"push", reference, filePath + ":" + mediaType, "--disable-path-validation"})
	stdout := bytes.Buffer{}
	cmds.SetOut(&stdout)
	stderr := bytes.Buffer{}
	cmds.SetErr(&stderr)
	return cmds.ExecuteContext(ctx)
}

func (o OrasSource) pushCopiedCommand(ctx context.Context, url string, mediaType string, contentReader io.Reader) (digest string, err error) {
	contents, err := io.ReadAll(contentReader)
	if err != nil {
		return "", err
	}

	repo, err := remote.NewRepository(url)
	if err != nil {
		return "", err
	}

	if o.PlainHTTP {
		repo.PlainHTTP = true
	} else {
		repo.PlainHTTP = false
	}
	// want tag:
	tag := repo.Reference.Reference

	// use the exact digest to tag
	repo.Reference.Reference = ""

	descriptor, err := oras.PushBytes(ctx, repo, mediaType, contents)
	if err != nil {
		return descriptor.Digest.String(), err
	}

	//packOpts := oras.PackManifestOptions{
	//	ConfigAnnotations:   annotations[option.AnnotationConfig],
	//	ManifestAnnotations: annotations[option.AnnotationManifest],
	//}

	repo.Reference.Reference = descriptor.Digest.String()
	descriptor, err = oras.Tag(ctx, repo, url+"@"+descriptor.Digest.String(), tag)
	if err != nil {
		return descriptor.Digest.String(), err
	}

	//descriptor, err = oras.TagBytes(ctx, repo, mediaType, contents, repo.Reference.Reference)

	log.Errorf("descriptor: %+v", descriptor)

	return descriptor.Digest.String(), err
}

func trimScheme(url string, scheme string) string {
	if !strings.HasSuffix(scheme, ":") {
		scheme += ":"
	}
	if strings.HasPrefix(url, scheme) {
		u := url[len(scheme):]
		if len(u) > 0 && u[0] == ':' {
			for len(u) > 0 && u[0] == '/' {
				u = u[1:]
			}
			return u
		}
	}
	return url
}

type pullOptions struct {
	//option.Cache
	//option.Common
	//option.Platform
	//option.Target
	//option.Format

	concurrency       int
	KeepOldFiles      bool
	IncludeSubject    bool
	PathTraversal     bool
	Output            string
	ManifestConfigRef string
}

func (o OrasSource) Pull(ctx context.Context, url string) (io.Reader, error) {
	// trim oras:// prefix
	url = trimScheme(url, "oras")
	return o.pullEmbeddedCommand(ctx, url)
}

func (o OrasSource) pullEmbeddedCommand(ctx context.Context, url string) (io.Reader, error) {
	url = strings.TrimPrefix(url, "oras://")

	cmds := root.New()
	tmpDir, err := os.MkdirTemp("", "pull-contents-")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	tmpDir, err = filepath.Abs(tmpDir)
	if err != nil {
		return nil, err
	}
	tmpFilePath, err := filepath.Abs(filepath.Join(tmpDir, "tmp"))
	if err != nil {
		return nil, err
	}
	tmpFile, err := os.OpenFile(tmpFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	closeOrLog(tmpFilePath, tmpFile)

	// push localhost:5000/hello:v1 hi.txt:application/vnd.me.hi
	cmds.SetArgs([]string{"pull", url, "-o", tmpDir})
	stdout := bytes.Buffer{}
	cmds.SetOut(&stdout)
	stderr := bytes.Buffer{}
	cmds.SetErr(&stderr)
	err = cmds.ExecuteContext(ctx)
	return tmpFile, err
}

func (o OrasSource) pullOrasCommandCopied(ctx context.Context, url string) (io.Reader, error) {
	repo, err := remote.NewRepository(url)
	if err != nil {
		return nil, err
	}

	if o.PlainHTTP {
		repo.PlainHTTP = true
	} else {
		repo.PlainHTTP = false
	}

	ref := repo.Reference.Reference
	repo.Reference.Reference = ""

	dst := memory.New()
	// Copy
	descriptor, err := oras.Copy(ctx, repo, ref, dst, ref, oras.DefaultCopyOptions)
	log.Errorf("descriptor1: %+v", descriptor)

	descriptor, reader, err := oras.Fetch(ctx, repo, ref, oras.FetchOptions{
		ResolveOptions: oras.ResolveOptions{
			TargetPlatform:   nil,
			MaxMetadataBytes: 8 * 1024, // TODO ?
		},
	})

	log.Errorf("descriptor2: %+v", descriptor)

	return reader, err
}

func (o OrasSource) List(ctx context.Context, filter string) (urls []string, listErr error) {
	//var dst content.Storage
	//
	//oras.CopyGraph(ctx, src, dst, root, oras.CopyGraphOptions{})

	return nil, fmt.Errorf("not implemented")
}

var _ Source = (*OrasSource)(nil)
