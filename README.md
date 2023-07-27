# gohlslib

[![Test](https://github.com/bluenviron/gohlslib/workflows/test/badge.svg)](https://github.com/bluenviron/gohlslib/actions?query=workflow:test)
[![Lint](https://github.com/bluenviron/gohlslib/workflows/lint/badge.svg)](https://github.com/bluenviron/gohlslib/actions?query=workflow:lint)
[![Go Report Card](https://goreportcard.com/badge/github.com/bluenviron/gohlslib)](https://goreportcard.com/report/github.com/bluenviron/gohlslib)
[![CodeCov](https://codecov.io/gh/bluenviron/gohlslib/branch/main/graph/badge.svg)](https://app.codecov.io/gh/bluenviron/gohlslib/branch/main)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/bluenviron/gohlslib)](https://pkg.go.dev/github.com/bluenviron/gohlslib#pkg-index)

HLS client and muxer library for the Go programming language.

Go &ge; 1.18 is required.

This library was forked from [MediaMTX](https://github.com/aler9/rtsp-simple-server), it is currently in alpha stage and implements only the features strictly needed by MediaMTX, but the aim is implementing a wide range of features that allow to read and generate HLS streams.

Client features:

|name|state|
|----|-----|
|Read MPEG-TS streams|OK|
|Read fMP4 streams|OK|
|Read Low-latency streams|TODO|
|Read H264 tracks|OK|
|Read H265 tracks|OK|
|Read MPEG-4 audio (AAC) tracks|OK|
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
|Write MPEG-4 audio (AAC) tracks|OK|
|Write Opus tracks|OK|
|Save generated segments on disk|OK|
|Generate multi-variant streams|TODO|

General features:

|name|state|
|----|-----|
|Parse and produce M3U8 playlists|OK|
|Examples|OK|

## Table of contents

* [Examples](#examples)
* [API Documentation](#api-documentation)
* [Standards](#standards)
* [Related projects](#related-projects)

## Examples

* [playlist-parser](examples/playlist-parser/main.go)
* [client](examples/client/main.go)
* [muxer](examples/muxer/main.go)

## API Documentation

https://pkg.go.dev/github.com/bluenviron/gohlslib#pkg-index

## Standards

* [RFC2616, HTTP 1.1](https://datatracker.ietf.org/doc/html/rfc2616)
* [RFC8216, HLS](https://datatracker.ietf.org/doc/html/rfc8216)
* [HLS v2](https://datatracker.ietf.org/doc/html/draft-pantos-hls-rfc8216bis)
* [Opus in MP4/ISOBMFF](https://opus-codec.org/docs/opus_in_isobmff.html)
* [Codec standards](https://github.com/bluenviron/mediacommon#standards)
* [Golang project layout](https://github.com/golang-standards/project-layout)

## Related projects

* [MediaMTX](https://github.com/bluenviron/mediamtx)
* [gortsplib](https://github.com/bluenviron/gortsplib)
* [mediacommon](https://github.com/bluenviron/mediacommon)
