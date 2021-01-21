package main

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gen2brain/go-unarr"
)

var excludeList = []string{
	"zzz-nahga-empire.jpg",
	"page.jpg",
	"page (newcomic.org).jpg",
	"zzz LDK6 zzz",
	"zzz K6 V1 zzz",
	"z_pitt",
	"zzZone2",
	"zSoU-Nerd",
	"zzzDQzzz",
	"zWater",
	"zzzNeverAngel-Empire",
}

var excludeOff = false

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	src := os.Args[1]
	targetDir := filepath.Dir(src)

	if len(os.Args) >= 3 {
		targetDir = os.Args[2]
	}

	if len(os.Args) == 4 {
		excludeOff = true
	}

	start := time.Now()

	if fileExist(src) { // if src is file
		if e := pack(src, targetDir); e != nil {
			panic(e)
		}
		duration := time.Since(start)
		fmt.Println("cost: ", duration)

		return
	}

	if !dirExist(targetDir) {
		if e := os.MkdirAll(targetDir, 0666); e != nil {
			panic(e)
		}
	}

	if !dirExist(src) {
		panic("target path is invalid: " + src)
	}

	// if src is dir, walk through
	e := filepath.Walk(src, func(fPath string, info os.FileInfo, err error) error {
		if info.IsDir() {
			rel, _ := filepath.Rel(src, fPath)

			if rel == "." { // skip root src dir
				return nil
			}

			// create same sub dir in targetDir
			newDir := filepath.Join(targetDir, rel)
			e := os.MkdirAll(newDir, 0666)
			if e != nil {
				return e
			}

			return nil
		}

		if !isComic(fPath) { // skip non-comic files
			return nil
		}

		rel, _ := filepath.Rel(src, filepath.Dir(fPath))
		newDir := filepath.Join(targetDir, rel)
		if e := pack(fPath, newDir); e != nil {
			fmt.Printf("convert failed, file: %s, error: %s\n", fPath, e)
		}

		return nil
	})

	duration := time.Since(start)
	fmt.Println("cost: ", duration)

	if e != nil {
		panic(e)
	}
}

func pack(src, targetDir string) error {
	fmt.Println("convert:", src)

	baseName := filepath.Base(src)
	ext := filepath.Ext(baseName)
	newName := strings.TrimSuffix(baseName, ext) + ".cbt"
	target := filepath.Join(targetDir, newName)

	return packArc(src, target)
}

func packArc(src, target string) error {
	ar, e := unarr.NewArchive(src)
	if e != nil {
		return e
	}
	defer ar.Close()

	f, e := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	if e != nil {
		return e
	}

	wr := tar.NewWriter(f)
	defer wr.Close()

	var previouseTime time.Time
	previousName := ""

	for e == nil {
		if e = ar.Entry(); e != nil {
			break
		}

		name := filepath.Base(ar.Name())

		// TODO unarr lib ignore dir entry in archive file
		if !isImage(name) {
			continue
		}

		// TODO unrar doesn't checksum
		data, e := ar.ReadAll()
		if e != nil {
			fmt.Printf("read file %s failed in %s, error:%s\n", name, src, e)
			continue
		}

		// backup excluded file
		if isExcluded(name, previousName, ar.ModTime(), previouseTime) {
			ext := filepath.Ext(target)
			target := strings.TrimSuffix(target, ext)
			backup := target + "_" + name
			e := ioutil.WriteFile(backup, data, 0666)
			if e != nil {
				fmt.Printf("backup excluded file failed:%s, error:%s\n", name, e)
			}
			continue
		}

		previousName = name
		previouseTime = ar.ModTime()

		h := &tar.Header{
			Name:    name,
			Mode:    int64(0666),
			Size:    int64(len(data)),
			ModTime: ar.ModTime(),
		}
		if e := wr.WriteHeader(h); e != nil {
			return fmt.Errorf("write .cbt header failed, file:%s, name:%s, error:%w\n",
				src, name, e)
		}
		if _, e := wr.Write(data); e != nil {
			return fmt.Errorf("write .cbt content failed, file:%s, name:%s, error:%w\n",
				src, name, e)
		}
	}

	if e != nil && e != io.EOF {
		return e
	}

	return nil
}

func isImage(name string) bool {
	ext := filepath.Ext(name)
	ext = strings.ToLower(ext)
	if ext == ".jpeg" || ext == ".jpg" || ext == ".png" || ext == ".webp" {
		return true
	}
	if ext == ".bmp" || ext == ".gif" || ext == ".tga" {
		fmt.Println(name)
		return true
	}

	return false
}

func isComic(name string) bool {
	ext := filepath.Ext(name)
	ext = strings.ToLower(ext)
	return ext == ".cbr" || ext == ".cbz" || ext == ".cbt" ||
		ext == ".rar" || ext == ".zip" || ext == ".tar"
}

func isExcluded(name, previousName string, currentTime, previousTime time.Time) bool {
	name = strings.ToLower(name)
	ext := filepath.Ext(name)
	fname := strings.TrimSuffix(name, ext)

	if strings.Index(name, "zz") == 0 ||
		strings.Index(name, "z_") == 0 ||
		strings.Index(name, "xxxx") == 0 ||
		strings.HasSuffix(fname, "tag") {
		return true
	}

	// check if name in excluded list
	for _, str := range excludeList {
		str = strings.ToLower(str)
		if strings.Contains(name, str) {
			return true
		}
	}

	if excludeOff {
		return false
	}

	bs := []byte(fname)
	i := 0
	for _, char := range bs {
		if char >= '0' && char <= '9' {
			i++
		}
	}
	if i < 2 { // if number char counts less than 2, it should be excludeded
		return true
	}

	if previousTime.Unix() < 0 {
		return false
	}
	// if 2 file modetime duration is greater than 20 days
	if math.Abs(float64(currentTime.Unix()-previousTime.Unix())) > 20*3600*24 {
		fmt.Println("duration > 20 days:", name)
		return true
	}

	if previousName == "" {
		return false
	}
	// if 2 filesname length are very close, return false
	if math.Abs(float64(len(name)-len(previousName))) < 2 {
		return false
	}
	if math.Abs(float64(len(name)-len(previousName))) > 5 {
		return true
	}

	// if one of last 2 chars of filename is number,it should be comic image
	char1 := bs[len(bs)-1]
	char2 := bs[len(bs)-2]
	if (char1 >= '0' && char1 <= '9') || (char2 >= '0' && char2 <= '9') &&
		(previousName != "" && math.Abs(float64(len(name)-len(previousName))) < 7) {
		return false
	}

	return false
}

func dirExist(dirName string) bool {
	info, err := os.Stat(dirName)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func fileExist(fileName string) bool {
	info, e := os.Stat(fileName)
	if os.IsNotExist(e) {
		return false
	}
	return !info.IsDir()
}
