# gohlslib

HLS client and muxer library for the Go programming language.

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
