//go:build !goverter

package docker

func init() {
	conv = &ConverterImpl{}
}
