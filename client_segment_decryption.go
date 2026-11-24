package gohlslib

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bluenviron/mediacommon/v2/pkg/limitedreader"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist"
)

const clientMaxInboundKeySize = 1 * 1024

func segmentIV(key *playlist.MediaKey, sequenceNo int) ([]byte, error) {
	iv := make([]byte, aes.BlockSize)

	if key.IV == "" {
		binary.BigEndian.PutUint64(iv[8:], uint64(sequenceNo))
		return iv, nil
	}

	v := strings.TrimPrefix(strings.TrimPrefix(key.IV, "0x"), "0X")

	if len(v) > hex.EncodedLen(aes.BlockSize) {
		return nil, fmt.Errorf("invalid IV: %v", key.IV)
	}

	v = strings.Repeat("0", hex.EncodedLen(aes.BlockSize)-len(v)) + v

	_, err := hex.Decode(iv, []byte(v))
	if err != nil {
		return nil, fmt.Errorf("invalid IV: %v", key.IV)
	}

	return iv, nil
}

func decryptSegmentAES128(payload []byte, key []byte, iv []byte) ([]byte, error) {
	if len(payload) == 0 || len(payload)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("invalid encrypted payload size: %d", len(payload))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	decrypted := make([]byte, len(payload))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(decrypted, payload)

	padding := decrypted[len(decrypted)-1]
	if padding == 0 || padding > aes.BlockSize {
		return nil, fmt.Errorf("invalid padding size: %d", padding)
	}

	for _, v := range decrypted[len(decrypted)-int(padding):] {
		if v != padding {
			return nil, fmt.Errorf("invalid padding")
		}
	}

	return decrypted[:len(decrypted)-int(padding)], nil
}

func (d *clientStreamDownloader) downloadKey(errGroupCtx context.Context, uri string) ([]byte, error) {
	key, ok := d.keys[uri]
	if ok {
		return key, nil
	}

	u, err := clientAbsoluteURL(d.playlistURL, uri)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(errGroupCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	d.onRequest(req)

	res, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", res.StatusCode)
	}

	key, err = io.ReadAll(&limitedreader.Reader{R: res.Body, N: clientMaxInboundKeySize})
	if err != nil {
		return nil, err
	}

	if len(key) != aes.BlockSize {
		return nil, fmt.Errorf("invalid key size: %d", len(key))
	}

	d.keys = map[string][]byte{uri: key}

	return key, nil
}

func (d *clientStreamDownloader) decryptSegment(
	errGroupCtx context.Context,
	seg *playlist.MediaSegment,
	sequenceNo int,
	payload []byte,
) ([]byte, error) {
	if seg.Key.Method != playlist.MediaKeyMethodAES128 {
		return nil, fmt.Errorf("unsupported encryption method: %v", seg.Key.Method)
	}

	key, err := d.downloadKey(errGroupCtx, seg.Key.URI)
	if err != nil {
		return nil, err
	}

	iv, err := segmentIV(seg.Key, sequenceNo)
	if err != nil {
		return nil, err
	}

	return decryptSegmentAES128(payload, key, iv)
}
