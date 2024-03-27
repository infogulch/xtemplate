package xtemplate

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"path"
)

type dotXProvider struct {
	instance *Instance
}

func (p dotXProvider) Value(Request) (any, error) {
	return DotX(p), nil
}

func (dotXProvider) Cleanup(_ any, err error) error {
	if errors.As(err, &ReturnError{}) {
		return nil
	}
	return err
}

var _ CleanupDotProvider = dotXProvider{}

// DotX is used as the field at `.X` in all template invocations.
type DotX struct {
	instance *Instance
}

// StaticFileHash returns the `sha-384` hash of the named asset file to be used
// for integrity or caching behavior.
func (d DotX) StaticFileHash(urlpath string) (string, error) {
	urlpath = path.Clean("/" + urlpath)
	fileinfo, ok := d.instance.files[urlpath]
	if !ok {
		return "", fmt.Errorf("file does not exist: '%s'", urlpath)
	}
	return fileinfo.hash, nil
}

// Template invokes the template `name` with the given `dot` value, returning
// the result as a html string.
func (c DotX) Template(name string, dot any) (template.HTML, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	t := c.instance.templates.Lookup(name)
	if t == nil {
		return "", fmt.Errorf("failed to lookup template name: '%s'", name)
	}
	if err := t.Execute(buf, dot); err != nil {
		return "", fmt.Errorf("failed to execute template '%s': %w", name, err)
	}
	return template.HTML(buf.String()), nil
}

// Func returns a function by name to call manually. Useful in combination with
// the `call` and `try` funcs.
func (c DotX) Func(name string) any {
	return c.instance.funcs[name]
}

// ReturnError is a sentinel value that indicates a successful/normal exit but
// causes template execution to stop immediately. Used by funcs and dot field
// methods to perform custom actions.
type ReturnError struct{}

func (ReturnError) Error() string { return "returned" }

var _ error = ReturnError{}
