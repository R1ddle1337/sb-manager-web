package web

import "embed"

// Files contains the complete UI so the release is a single static binary.
//
//go:embed templates/* static/*
var Files embed.FS
