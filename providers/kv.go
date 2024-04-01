package providers

type DotKV struct {
	m map[string]string
}

func (d DotKV) Value(key string) string {
	return d.m[key]
}
