# gohlslib

[![Test](https://github.com/bluenviron/gohlslib/workflows/test/badge.svg)](https://github.com/bluenviron/gohlslib/actions?query=workflow:test)
[![Lint](https://github.com/bluenviron/gohlslib/workflows/lint/badge.svg)](https://github.com/bluenviron/gohlslib/actions?query=workflow:lint)
[![Go Report Card](https://goreportcard.com/badge/github.com/bluenviron/gohlslib)](https://goreportcard.com/report/github.com/bluenviron/gohlslib)
[![CodeCov](https://codecov.io/gh/bluenviron/gohlslib/branch/main/graph/badge.svg)](https://app.codecov.io/gh/bluenviron/gohlslib/branch/main)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/bluenviron/gohlslib)](https://pkg.go.dev/github.com/bluenviron/gohlslib#pkg-index)

HLS client and muxer library for the Go programming language.

Go &ge; 1.18 is required.

This library was splitted from [MediaMTX](https://github.com/aler9/rtsp-simple-server), it is currently in alpha stage and implements only the features strictly needed by MediaMTX, but the aim is implementing a wide range of features that allow to read and generate HLS streams.

Client features:

|name|state|
|----|-----|
|Read MPEG-TS streams|OK|
|Read fMP4 streams|OK|
|Read Low-latency streams|TODO (needs a new M3U8 parser)|
|Read H264 tracks|OK|
|Read H265 tracks|OK|
|Read MPEG4 Audio (AAC) tracks|OK|
|Read Opus tracks|OK|
|Read a given variant|TODO (currently a single variant is read)|

Muxer features:

|name|state|
|----|-----|
|Generate MPEG-TS streams|OK|
|Generate fMP4 streams|OK|
|Generate Low-latency streams|OK|
|Write H264 tracks|OK|
|Write H265 tracks|OK|
|Write MPEG4 Audio (AAC) tracks|OK|
|Write Opus tracks|OK|
|Generate multi-variant streams|TODO|
|Save on disk generated segments|TODO|

General features:

|name|state|
|----|-----|
|Examples|TODO|
|Detach from gortsplib|TODO|
