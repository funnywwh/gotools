package main

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/astaxie/beego/logs"
)

var releasePath = flag.String("o", "./release", "输出目录")
var gitinit = flag.Bool("i", true, "是否执行git submodule init")
var gitupdate = flag.Bool("u", true, "是否执行git submodule update")
var gitpull = flag.Bool("pull", false, "是否对指定的模块执行git pull origin master")
var gitremote = flag.String("remote", "origin", "remote repo of pull ")
var gitbranch = flag.String("branch", "master", "branch of repo of pull")
var inonzip = flag.Bool("zip", false, "zip all module files into one zip file")
var runmode = flag.String("runmode", "prod", "runmode for all service")
var git Git

var regRunMode = regexp.MustCompile(`(?i:runmode\s*=\s*\S+)`)

func main() {
	flag.Parse()
	builder, err := NewBuilder(*releasePath)
	if err != nil {
		logs.Error(err)
		os.Exit(1)
	}
	if *gitpull {
		var git Git
		git.PullSubModule(flag.Args()...)
		return
	}
	defer func() {
		if *inonzip {
			if builder.zipw != nil {
				builder.zipw.Close()
			}
			if builder.f != nil {
				builder.f.Close()
			}
		}

	}()
	builder.Build(flag.Args()...)
}

type Builder struct {
	releasePath  string
	gopath       string //也是build的工作目录
	serviceFiles map[string]interface{}
	zipw         *zip.Writer
	f            *os.File
}

func NewBuilder(releasePath string) (builder *Builder, err error) {
	builder = &Builder{
		releasePath: releasePath,
	}
	builder.gopath, err = os.Getwd()
	return
}

//编译服务
func (this *Builder) Build(serviceList ...string) (err error) {
	err = this.LoadReleaseConf()
	if err != nil {
		logs.Error(err)
		return
	}
	this.SetEnv()
	//remove release path
	os.RemoveAll(this.releasePath)

	os.Mkdir(this.releasePath, 0777)
	logs.Debug("gopath:%s", this.gopath)
	os.Chdir(this.gopath)
	if *gitinit {
		err = git.SubModuleInit()
		if err != nil {
			logs.Error(err)
			return
		}
	}
	if *gitupdate {
		git.SubModuleUpdate()
		if err != nil {
			logs.Error(err)
			return
		}
	}
	if len(serviceList) == 0 {
		for k, _ := range this.serviceFiles {
			serviceList = append(serviceList, k)
		}
	}
	for _, serviceName := range serviceList {
		err = this.BuildService(serviceName)
		if err != nil {
			logs.Error(err)
			return
		}
		os.Chdir(this.gopath)
	}
	return
}
func (this *Builder) LoadReleaseConf() (err error) {
	jsonpath := filepath.Join(this.gopath, "release.json")
	f, err := os.Open(jsonpath)
	if err != nil {
		logs.Error(err)
		return
	}
	defer f.Close()
	jsonbs, err := ioutil.ReadAll(f)
	if err != nil {
		logs.Error(err)
		return
	}
	err = json.Unmarshal(jsonbs, &this.serviceFiles)
	if err != nil {
		logs.Error(err)
		return
	}
	return
}

//编译指定服务
//serviceName 服务名称，生成的程序文件名
//	service的源码目录在src/<serviceName>下
func (this *Builder) BuildService(serviceName string) (err error) {
	logs.Debug("buildservice:%s", serviceName)
	err = os.Chdir(filepath.Join("src", serviceName))
	if err != nil {
		logs.Error(err)
		return
	}
	var gobuild = true
	if v, ok := this.GetServiceFiles(serviceName)["gobuild"]; ok {
		gobuild = v.(bool)
	}
	if gobuild {
		err = this.GoBuild()
		if err != nil {
			logs.Error(err)
			return
		}
		defer os.Remove(serviceName)
	}

	this.Zip(serviceName)
	return
}

func (this *Builder) GoBuild() (err error) {
	err = Exec("go", "build", "-ldflags", "-s -w")
	return
}
func (this *Builder) GetServiceFiles(serviceName string) (files map[string]interface{}) {
	files = make(map[string]interface{})
	if im, ok := this.serviceFiles[serviceName]; ok {
		if m, ok := im.(map[string]interface{}); ok {
			for k, v := range m {
				files[k] = v
			}
		}

	}
	return
}

//将部署文件一超打包成zip文件
//serviceName 将输出到releasePath下的serviceName目录下
func (this *Builder) Zip(serviceName string) (err error) {
	logs.Debug("zip:%s", serviceName)
	files := this.GetServiceFiles(serviceName)
	if err != nil {
		logs.Error(err)
		return
	}
	var zipf *zip.Writer

	if *inonzip {
		if this.zipw == nil {
			var f *os.File
			f, err = os.OpenFile(filepath.Join(this.gopath, this.releasePath, "release.zip"), os.O_CREATE|os.O_RDWR, 0666)
			if err != nil {
				logs.Error(err)
				return
			}
			zipf = zip.NewWriter(f)
			this.zipw = zipf
		} else {
			zipf = this.zipw
		}
	} else {
		var f *os.File
		f, err = os.OpenFile(filepath.Join(this.gopath, this.releasePath, serviceName+".zip"), os.O_CREATE|os.O_RDWR, 0666)
		if err != nil {
			logs.Error(err)
			return
		}
		defer f.Close()
		zipf = zip.NewWriter(f)
		defer zipf.Close()
	}

	for file, v := range files {
		var perm string
		var ok bool
		if perm, ok = v.(string); !ok {
			continue
		}
		fh := &zip.FileHeader{
			Name:   fmt.Sprintf("%s/%s", serviceName, file),
			Method: zip.Deflate,
		}
		var mode os.FileMode
		_, err = fmt.Sscan(perm, &mode)
		if err != nil {
			logs.Error(err)
			return
		}
		fh.SetMode(mode)
		var w io.Writer
		w, err = zipf.CreateHeader(fh)
		if err != nil {
			logs.Error(err)
			return
		}
		//修复conf/app.conf的runmode
		var r *os.File
		r, err = os.Open(file)
		if err != nil {
			logs.Error(err)
			return
		}
		defer r.Close()
		if _, ok := this.GetServiceFiles(serviceName)["conf/app.conf"]; ok {
			//替换runmode
			bufr := bufio.NewReader(r)
			var line string
			for {
				line, err = bufr.ReadString('\n')
				if err != nil {
					break
				}
				if regRunMode.MatchString(line) {
					logs.Info("%s", line)
					line = fmt.Sprintf("runmode=%s\n", *runmode)
				}
				if len(line) > 0 {
					w.Write([]byte(fmt.Sprintf("%s", line)))
				}
			}
		} else {
			_, err = io.Copy(w, r)
			if err != nil {
				logs.Error(err)
				return
			}

		}
	}
	return
}

//设置环境变量
func (this *Builder) SetEnv() {
	os.Setenv("GOPATH", this.gopath)
	os.Setenv("GOARCH", "amd64")
	os.Setenv("GOOS", "linux")
	os.Setenv("CGO_ENABLED", "0")
	os.Setenv("PATH", os.ExpandEnv(`$GOROOT\bin;$PATH`))
}

func Exec(app string, args ...string) (err error) {
	cmd := exec.Command(app,
		args...,
	)
	cmd.Env = os.Environ()

	logs.Debug("%v", cmd.Args)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logs.Error(err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		logs.Error(err)
		return
	}
	loopreader := func(r io.ReadCloser, w io.Writer) {
		defer func() {
			recover()
			r.Close()
		}()
		linereader := bufio.NewReader(r)
		for {
			linebs, _, err := linereader.ReadLine()
			if err != nil {
				if err != io.EOF {
					return
				}
			}
			if len(linebs) > 0 {
				fmt.Fprintln(w, string(linebs))
			}
		}
	}

	go loopreader(stdout, os.Stdout)
	go loopreader(stderr, os.Stderr)
	err = cmd.Run()
	if err != nil {
		logs.Error("err:%v", err)
		return
	}

	return
}

type Git struct {
}

//git submodule init
func (this *Git) SubModuleInit() (err error) {
	logs.Debug("SubModuleInit")
	err = Exec("git", "submodule", "init")
	return
}

//git submodule update
func (this *Git) SubModuleUpdate() (err error) {
	logs.Debug("SubModuleUpdate")
	err = Exec("git", "submodule", "update")
	return
}

//git submodule pull
func (this *Git) PullSubModule(models ...string) (err error) {
	logs.Debug("PullSubModule")
	err = Exec("git", "submodule",
		"foreach", "git", "pull", *gitremote, *gitbranch)
	return
}
