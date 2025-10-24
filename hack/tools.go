// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build tools
// +build tools

// This package imports things required by build scripts, to force `go mod` to see them as dependencies
package tools

import (
	_ "github.com/MakeNowJust/heredoc"
	_ "github.com/gardener/gardener/cmd/gardener-apiserver/app"
	_ "github.com/gardener/gardener/hack"
	_ "github.com/valyala/fastjson"
	_ "k8s.io/cluster-bootstrap"

	// TODO: Remove after conflict between `ENSURE_CAPI_MOD` (cluster-api) and `go mod tidy` with this indirect dependency
	//  has been resolved.
	_ "github.com/google/gofuzz"
)
