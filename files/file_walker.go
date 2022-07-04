package files

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"fmt"
)

// File structure representing files and folders with their accumulated sizes
type File struct {
	Name   string
	Parent *File
	Size   int64
	IsDir  bool
	Files  []*File
}

// Path builds a file system location for given file
func (f *File) Path() string {
	if f.Parent == nil {
		return f.Name
	}
	return filepath.Join(f.Parent.Path(), f.Name)
}

// UpdateSize goes through subfiles and subfolders and accumulates their size
func (f *File) UpdateSize() {
	if !f.IsDir {    //是文件直接返回到调用的地方
		return
	}
	var size int64
	for _, child := range f.Files {   
		child.UpdateSize()
		size += child.Size //累加文件的大小
	}
	f.Size = size
}

// ReadDir function can return list of files for given folder path
type ReadDir func(dirname string) ([]os.FileInfo, error)

// ShouldIgnoreFolder function decides whether a folder should be ignored
type ShouldIgnoreFolder func(absolutePath string) bool

func ignoringReadDir(shouldIgnore ShouldIgnoreFolder, originalReadDir ReadDir) ReadDir {
	return func(path string) ([]os.FileInfo, error) {
		if shouldIgnore(path) {
			return []os.FileInfo{}, nil  //如果需要忽略目录，则返回空的FileInfo，否则返回原始读的目录
		}
		return originalReadDir(path)
	}
}

// WalkFolder will go through a given folder and subfolders and produces file structure
// with aggregated file sizes
func WalkFolder(
	path string,
	readDir ReadDir,
	ignoreFunction ShouldIgnoreFolder,
	progress chan<- int,
) *File {
	var wg sync.WaitGroup
	c := make(chan bool, 2*runtime.NumCPU())  //runtime.NumCPU()返回可用cpu数，创建带缓冲通道
	root := walkSubFolderConcurrently(path, nil, ignoringReadDir(ignoreFunction, readDir), c, &wg, progress) //一开始默认的父目录为空
	wg.Wait()
	close(progress)
	root.UpdateSize()
	return root
}

func walkSubFolderConcurrently(
	path string,
	parent *File,
	readDir ReadDir,
	c chan bool,
	wg *sync.WaitGroup,
	progress chan<- int,
) *File {
	result := &File{}  //初始化一个File对象
	entries, err := readDir(path)    //读取path
	if err != nil {
		log.Println(err)
		return result
	}
	dirName, name := filepath.Split(path)  
	result.Files = make([]*File, 0, len(entries)) //这个为什么这么做呢？
	fmt.Printf("reslult is %v",result)
	numSubFolders := 0
	defer updateProgress(progress, &numSubFolders)
	var mutex sync.Mutex
	for _, entry := range entries {
		if entry.IsDir() {
			numSubFolders++
			subFolderPath := filepath.Join(path, entry.Name())
			wg.Add(1)
			go func() {
				c <- true
				subFolder := walkSubFolderConcurrently(subFolderPath, result, readDir, c, wg, progress) //递归处理
				mutex.Lock()
				result.Files = append(result.Files, subFolder)
				mutex.Unlock()
				<-c
				wg.Done()
			}()
		} else {
			size := entry.Size()
			file := &File{
				entry.Name(),
				result,
				size,
				false,
				[]*File{},
			}
			mutex.Lock()
			result.Files = append(result.Files, file)
			mutex.Unlock()
		}
	}
	if parent != nil {
		result.Name = name
		result.Parent = parent
	} else {
		// Root dir
		// TODO unit test this Join
		result.Name = filepath.Join(dirName, name)
	}
	result.IsDir = true
	return result
}

func updateProgress(progress chan<- int, count *int) {
	if *count > 0 {
		progress <- *count
	}
}
