//go:build noui

package main

import "net/http"

func registerUI(_ *http.ServeMux) {}
