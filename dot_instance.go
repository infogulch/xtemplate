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

type instanceDotProvider struct {
	instance *Instance
}

func (instanceDotProvider) Type() reflect.Type { return reflect.TypeOf(InstanceDot{}) }

func (p instanceDotProvider) Value(log *slog.Logger, sctx context.Context, w http.ResponseWriter, r *http.Request) (reflect.Value, error) {
	return reflect.ValueOf(InstanceDot(p)), nil
}

func (instanceDotProvider) Cleanup(_ reflect.Value, err error) error {
	if errors.As(err, &ReturnError{}) {
		return nil
	}
	return err
}

var _ CleanupDotProvider = instanceDotProvider{}

type InstanceDot struct {
	instance *Instance
}

func (d InstanceDot) StaticFileHash(urlpath string) (string, error) {
	urlpath = path.Clean("/" + urlpath)
	fileinfo, ok := d.instance.files[urlpath]
	if !ok {
		return "", fmt.Errorf("file does not exist: '%s'", urlpath)
	}
	return fileinfo.hash, nil
}

func (c InstanceDot) Template(name string, context any) (string, error) {
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

func (c InstanceDot) Func(name string) any {
	return c.instance.funcs[name]
}

// ReturnError is a sentinel value returned by the `return` template
// func that indicates a successful/normal exit but allows the template
// to exit early.
//
// If a custom func needs to stop template execution to immediately exit
// successfully, then it can return this value in its error param.
type ReturnError struct{}

func (ReturnError) Error() string { return "returned" }

var _ error = (*ReturnError)(nil)
