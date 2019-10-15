package utils

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
)

// BackupRequestBody returns a backup of the Request.Body, leaving it untouched
// so the passed Request.Body will be still readable.
func BackupRequestBody(req *http.Request) io.ReadCloser {
	content, _ := ioutil.ReadAll(req.Body)
	req.Body = ioutil.NopCloser(bytes.NewBuffer(content))
	return ioutil.NopCloser(bytes.NewBuffer(content))
}

// ReadResponseAndRestore reads the content of the passed Response.Body,
// and restores it to allow its reuse (e.g. when read from a nested Transport)
func ReadResponseAndRestore(resp *http.Response) ([]byte, error) {
	reader, content, err := readAndGetNew(resp.Body)
	if err != nil {
		return nil, err
	}

	resp.Body = reader
	return content, nil
}

// ReadRequestAndRestore reads the content of the Response.Body, and restores it
// to allow its reuse (e.g. when read from a nested Transport)
func ReadRequestAndRestore(req *http.Request) ([]byte, error) {
	reader, content, err := readAndGetNew(req.Body)
	if err != nil {
		return nil, err
	}

	req.Body = reader
	return content, nil
}

func readAndGetNew(reader io.Reader) (io.ReadCloser, []byte, error) {
	content, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, nil, err
	}

	return ioutil.NopCloser(bytes.NewReader(content)), content, nil
}
