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
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

var _ = gc.Suite(&TarSuite{})

type TarSuite struct {
	testing.BaseSuite
	cwd       string
	testFiles []string
}

func (t *TarSuite) SetUpTest(c *gc.C) {
	t.cwd = c.MkDir()
	t.BaseSuite.SetUpTest(c)
}

func (t *TarSuite) createTestFiles(c *gc.C) {
	tarDirE := filepath.Join(t.cwd, "TarDirectoryEmpty")
	err := os.Mkdir(tarDirE, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	tarDirP := filepath.Join(t.cwd, "TarDirectoryPopulated")
	err = os.Mkdir(tarDirP, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	tarSubFile1 := filepath.Join(tarDirP, "TarSubFile1")
	tarSubFile1Handle, err := os.Create(tarSubFile1)
	c.Check(err, gc.IsNil)
	tarSubFile1Handle.WriteString("TarSubFile1")
	tarSubFile1Handle.Close()

	tarSubDir := filepath.Join(tarDirP, "TarDirectoryPopulatedSubDirectory")
	err = os.Mkdir(tarSubDir, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	tarFile1 := filepath.Join(t.cwd, "TarFile1")
	tarFile1Handle, err := os.Create(tarFile1)
	c.Check(err, gc.IsNil)
	tarFile1Handle.WriteString("TarFile1")
	tarFile1Handle.Close()

	tarFile2 := filepath.Join(t.cwd, "TarFile2")
	tarFile2Handle, err := os.Create(tarFile2)
	c.Check(err, gc.IsNil)
	tarFile2Handle.WriteString("TarFile2")
	tarFile2Handle.Close()
	t.testFiles = []string{tarDirE, tarDirP, tarFile1, tarFile2}

}

func (t *TarSuite) removeTestFiles(c *gc.C) {
	for _, removable := range t.testFiles {
		err := os.RemoveAll(removable)
		c.Assert(err, gc.IsNil)
	}
}

type expectedTarContents struct {
	Name string
	Body string
}

var testExpectedTarContents = []expectedTarContents{
	{"TarDirectoryEmpty", ""},
	{"TarDirectoryPopulated", ""},
	{"TarDirectoryPopulated/TarSubFile1", "TarSubFile1"},
	{"TarDirectoryPopulated/TarDirectoryPopulatedSubDirectory", ""},
	{"TarFile1", "TarFile1"},
	{"TarFile2", "TarFile2"},
}

// Assert thar contents checks that the tar[.gz] file provided contains the
// Expected files
// expectedContents: is a slice of the filenames with relative paths that are
// expected to be on the tar file
// tarFile: is the path of the file to be checked
func (t *TarSuite) assertTarContents(c *gc.C, expectedContents []expectedTarContents,
	tarFile string,
	compressed bool) {
	f, err := os.Open(tarFile)
	c.Assert(err, gc.IsNil)
	defer f.Close()
	var r io.Reader = f
	if compressed {
		r, err = gzip.NewReader(r)
		c.Assert(err, gc.IsNil)
	}

	tr := tar.NewReader(r)

	tarContents := make(map[string]string)
	// Iterate through the files in the archive.
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		c.Assert(err, gc.IsNil)
		buf, err := ioutil.ReadAll(tr)
		c.Assert(err, gc.IsNil)
		tarContents[hdr.Name] = string(buf)
	}
	for _, expectedContent := range expectedContents {
		fullExpectedContent := strings.TrimPrefix(expectedContent.Name, string(os.PathSeparator))
		body, ok := tarContents[fullExpectedContent]
		c.Log(tarContents)
		c.Log(expectedContents)
		c.Log(fmt.Sprintf("checking for presence of %q on tar file", fullExpectedContent))
		c.Assert(ok, gc.Equals, true)
		if expectedContent.Body != "" {
			c.Log("Also checking the file contents")
			c.Assert(body, gc.Equals, expectedContent.Body)
		}
	}

}

func (t *TarSuite) assertFilesWhereUntared(c *gc.C,
	expectedContents []expectedTarContents,
	tarOutputFolder string) {
	tarContents := make(map[string]string)
	var walkFn filepath.WalkFunc
	walkFn = func(path string, finfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		fileName := strings.TrimPrefix(path, tarOutputFolder)
		fileName = strings.TrimPrefix(fileName, string(os.PathSeparator))
		if fileName == "" {
			return nil
		}
		if finfo.IsDir() {
			tarContents[fileName] = ""
		} else {
			readable, err := os.Open(path)
			if err != nil {
				return err
			}
			defer readable.Close()
			buf, err := ioutil.ReadAll(readable)
			c.Assert(err, gc.IsNil)
			tarContents[fileName] = string(buf)
		}
		return nil
	}
	filepath.Walk(tarOutputFolder, walkFn)
	for _, expectedContent := range expectedContents {
		fullExpectedContent := strings.TrimPrefix(expectedContent.Name, string(os.PathSeparator))
		expectedPath := filepath.Join(tarOutputFolder, fullExpectedContent)
		_, err := os.Stat(expectedPath)
		c.Assert(err, gc.Equals, nil)
		body, ok := tarContents[fullExpectedContent]
		c.Log(fmt.Sprintf("checking for presence of %q on untar files", fullExpectedContent))
		c.Assert(ok, gc.Equals, true)
		if expectedContent.Body != "" {
			c.Log("Also checking the file contents")
			c.Assert(body, gc.Equals, expectedContent.Body)
		}
	}

}

func shaSumFile(c *gc.C, fileToSum string) string {
	f, err := os.Open(fileToSum)
	c.Assert(err, gc.IsNil)
	defer f.Close()
	shahash := sha1.New()
	_, err = io.Copy(shahash, f)
	c.Assert(err, gc.IsNil)
	return base64.StdEncoding.EncodeToString(shahash.Sum(nil))
}

// Tar

func (t *TarSuite) TestTarFilesUncompressed(c *gc.C) {
	t.createTestFiles(c)
	outputTar := filepath.Join(t.cwd, "output_tar_file.tar")
	trimPath := fmt.Sprintf("%s/", t.cwd)
	shaSum, err := TarFiles(t.testFiles, outputTar, trimPath, false)
	c.Check(err, gc.IsNil)
	fileShaSum := shaSumFile(c, outputTar)
	c.Assert(shaSum, gc.Equals, fileShaSum)
	t.removeTestFiles(c)
	t.assertTarContents(c, testExpectedTarContents, outputTar, false)
}

func (t *TarSuite) TestTarFilesCompressed(c *gc.C) {
	t.createTestFiles(c)
	outputTarGz := filepath.Join(t.cwd, "output_tar_file.tgz")
	trimPath := fmt.Sprintf("%s/", t.cwd)
	shaSum, err := TarFiles(t.testFiles, outputTarGz, trimPath, true)
	c.Check(err, gc.IsNil)

	fileShaSum := shaSumFile(c, outputTarGz)
	c.Assert(shaSum, gc.Equals, fileShaSum)

	t.assertTarContents(c, testExpectedTarContents, outputTarGz, true)
}

// UnTar

func (t *TarSuite) TestUnTarFilesUncompressed(c *gc.C) {
	t.createTestFiles(c)
	outputTar := filepath.Join(t.cwd, "output_tar_file.tar")
	trimPath := fmt.Sprintf("%s/", t.cwd)
	_, err := TarFiles(t.testFiles, outputTar, trimPath, false)
	c.Check(err, gc.IsNil)
	t.removeTestFiles(c)

	outputDir := filepath.Join(t.cwd, "TarOuputFolder")
	err = os.Mkdir(outputDir, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	UntarFiles(outputTar, outputDir, false)
	t.assertFilesWhereUntared(c, testExpectedTarContents, outputDir)
}

func (t *TarSuite) TestUntarTarFilesCompressed(c *gc.C) {
	t.createTestFiles(c)
	outputTarGz := filepath.Join(t.cwd, "output_tar_file.tgz")
	trimPath := fmt.Sprintf("%s/", t.cwd)
	_, err := TarFiles(t.testFiles, outputTarGz, trimPath, true)
	c.Check(err, gc.IsNil)
	t.removeTestFiles(c)

	outputDir := filepath.Join(t.cwd, "TarOuputFolder")
	err = os.Mkdir(outputDir, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	UntarFiles(outputTarGz, outputDir, true)
	t.assertFilesWhereUntared(c, testExpectedTarContents, outputDir)
}
