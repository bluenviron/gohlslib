# gohlslib

[![Test](https://github.com/bluenviron/gohlslib/workflows/test/badge.svg)](https://github.com/bluenviron/gohlslib/actions?query=workflow:test)
[![Lint](https://github.com/bluenviron/gohlslib/workflows/lint/badge.svg)](https://github.com/bluenviron/gohlslib/actions?query=workflow:lint)
[![Go Report Card](https://goreportcard.com/badge/github.com/bluenviron/gohlslib)](https://goreportcard.com/report/github.com/bluenviron/gohlslib)
[![CodeCov](https://codecov.io/gh/bluenviron/gohlslib/branch/main/graph/badge.svg)](https://app.codecov.io/gh/bluenviron/gohlslib/branch/main)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/bluenviron/gohlslib)](https://pkg.go.dev/github.com/bluenviron/gohlslib#pkg-index)

HLS client and muxer library for the Go programming language, written for [MediaMTX](https://github.com/bluenviron/mediamtx).

Go &ge; 1.19 is required.

Features:

* Client

  * Read MPEG-TS or fMP4 streams
  * Read tracks encoded with AV1, VP9, H265, H264, Opus, MPEG-4 Audio (AAC)

* Muxer

  * Generate MPEG-TS, fMP4, Low-latency streams
  * Write tracks encoded with AV1, VP9, H265, H264, Opus, MPEG-4 audio (AAC)
  * Save generated segments on disk

* General

  * Parse and produce M3U8 playlists
  * Examples

## Table of contents

* [Examples](#examples)
* [API Documentation](#api-documentation)
* [Specifications](#specifications)
* [Related projects](#related-projects)

## Examples

* [playlist-parser](examples/playlist-parser/main.go)
* [client](examples/client/main.go)
* [muxer](examples/muxer/main.go)

## API Documentation

[Click to open the API Documentation](https://pkg.go.dev/github.com/bluenviron/gohlslib#pkg-index)

## Specifications

|name|area|
|----|----|
|[RFC2616, HTTP 1.1](https://datatracker.ietf.org/doc/html/rfc2616)|protocol|
|[RFC8216, HLS](https://datatracker.ietf.org/doc/html/rfc8216)|protocol|
|[HLS v2](https://datatracker.ietf.org/doc/html/draft-pantos-hls-rfc8216bis)|protocol|
|[Codec specifications](https://github.com/bluenviron/mediacommon#specifications)|codecs|
|[Golang project layout](https://github.com/golang-standards/project-layout)|project layout|

## Related projects

* [MediaMTX](https://github.com/bluenviron/mediamtx)
* [gortsplib](https://github.com/bluenviron/gortsplib)
* [mediacommon](https://github.com/bluenviron/mediacommon)
