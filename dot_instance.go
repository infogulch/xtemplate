package xtemplate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"reflect"
)

type dotXProvider struct {
	instance *Instance
}

func (dotXProvider) Type() reflect.Type { return reflect.TypeOf(DotX{}) }

func (p dotXProvider) Value(log *slog.Logger, sctx context.Context, w http.ResponseWriter, r *http.Request) (reflect.Value, error) {
	return reflect.ValueOf(DotX(p)), nil
}

func (dotXProvider) Cleanup(_ reflect.Value, err error) error {
	if errors.As(err, &ReturnError{}) {
		return nil
	}
	return err
}

var _ CleanupDotProvider = dotXProvider{}

type DotX struct {
	instance *Instance
}

func (d DotX) StaticFileHash(urlpath string) (string, error) {
	urlpath = path.Clean("/" + urlpath)
	fileinfo, ok := d.instance.files[urlpath]
	if !ok {
		return "", fmt.Errorf("file does not exist: '%s'", urlpath)
	}
	return fileinfo.hash, nil
}

func (c DotX) Template(name string, context any) (string, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	t := c.instance.templates.Lookup(name)
	if t == nil {
		return "", fmt.Errorf("failed to lookup template name: '%s'", name)
	}
	if err := t.Execute(buf, context); err != nil {
		return "", fmt.Errorf("failed to execute template '%s': %w", name, err)
	}
	return buf.String(), nil
}

func (c DotX) Func(name string) any {
	return c.instance.funcs[name]
}

// ReturnError is a sentinel value that indicates a successful/normal exit but
// causes template execution to stop immediately. Used by funcs and dot field
// methods to perform custom actions.
type ReturnError struct{}

func (ReturnError) Error() string { return "returned" }

var _ error = ReturnError{}
