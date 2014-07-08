// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package tar

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.tar")

// TarFiles creates a tar archive at targetPath holding the files listed
// in fileList. If compress is true, the archive will also be gzip
// compressed.
func TarFiles(fileList []string, targetPath, strip string, compress bool) (shaSum string, err error) {
	shahash := sha1.New()
	if err := tarAndHashFiles(fileList, targetPath, strip, compress, shahash); err != nil {
		return "", err
	}
	// we use a base64 encoded sha1 hash, because this is the hash
	// used by RFC 3230 Digest headers in http responses
	encodedHash := base64.StdEncoding.EncodeToString(shahash.Sum(nil))
	return encodedHash, nil
}

func tarAndHashFiles(fileList []string, targetPath, strip string, compress bool, hashw io.Writer) (err error) {
	checkClose := func(w io.Closer) {
		if closeErr := w.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("error closing backup file: %v", closeErr)
		}
	}
	f, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("cannot create backup file %q", targetPath)
	}
	defer checkClose(f)

	w := io.MultiWriter(f, hashw)

	if compress {
		gzw := gzip.NewWriter(w)
		defer checkClose(gzw)
		w = gzw
	}

	tarw := tar.NewWriter(w)
	defer checkClose(tarw)
	for _, ent := range fileList {
		if err := writeContents(ent, strip, tarw); err != nil {
			return fmt.Errorf("backup failed: %v", err)
		}
	}
	return nil
}

// writeContents creates an entry for the given file
// or directory in the given tar archive.
func writeContents(fileName, strip string, tarw *tar.Writer) error {
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	fInfo, err := f.Stat()
	if err != nil {
		return err
	}
	h, err := tar.FileInfoHeader(fInfo, "")
	if err != nil {
		return fmt.Errorf("cannot create tar header for %q: %v", fileName, err)
	}
	h.Name = filepath.ToSlash(strings.TrimPrefix(fileName, strip))
	if err := tarw.WriteHeader(h); err != nil {
		return fmt.Errorf("cannot write header for %q: %v", fileName, err)
	}
	if !fInfo.IsDir() {
		if _, err := io.Copy(tarw, f); err != nil {
			return fmt.Errorf("failed to write %q: %v", fileName, err)
		}
		return nil
	}
	if !strings.HasSuffix(fileName, string(os.PathSeparator)) {
		fileName = fileName + string(os.PathSeparator)
	}

	for {
		names, err := f.Readdirnames(100)
		if len(names) == 0 && err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("error reading directory %q: %v", fileName, err)
		}
		for _, name := range names {
			if err := writeContents(filepath.Join(fileName, name), strip, tarw); err != nil {
				return err
			}
		}
	}

}

func UntarFiles(tarFile, outputFolder string, compressed bool) error {
	f, err := os.Open(tarFile)
	if err != nil {
		return fmt.Errorf("cannot open backup file %q: %v", tarFile, err)
	}
	defer f.Close()
	var r io.Reader = f
	if compressed {
		r, err = gzip.NewReader(r)
		if err != nil {
			return fmt.Errorf("cannot uncompress tar file %q: %v", tarFile, err)
		}
	}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		if err != nil {
			return fmt.Errorf("failed while reading tar header: %v", err)
		}
		buf := make([]byte, hdr.Size)
		buf, err = ioutil.ReadAll(tr)
		if err != nil {
			return fmt.Errorf("failed while reading tar contents: %v", err)
		}
		fullPath := filepath.Join(outputFolder, hdr.Name)
		if hdr.Typeflag == tar.TypeDir {
			if err = os.MkdirAll(fullPath, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("cannot extract directory %q: %v", fullPath, err)
			}
		} else {
			fh, err := os.Create(fullPath)
			if err != nil {
				return fmt.Errorf("some of the tar contents cannot be written to disk: %v", err)
			}
			_, err = fh.Write(buf)

			if err != nil {
				fh.Close()
				return fmt.Errorf("some of the tar contents cannot be written to disk: %v", err)
			}
			err = fh.Chmod(os.FileMode(hdr.Mode))
			fh.Close()
			if err != nil {
				return fmt.Errorf("cannot set proper mode on file %q: %v", fullPath, err)
			}

		}
	}
	return nil
}
