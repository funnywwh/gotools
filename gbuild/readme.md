编译多个git submodule的golang工程，并将输出文件打包成zip
```
{
	"conf":{//模块
		"gobuild":false,//false:不编译,没有设置，或者true编译
		"common.conf":"0666"//要打包的文件，路径为gbuild/conf/common.conf
	},
	"helloworld":{
		"helloworld":"0777",
		"conf/app.conf":"0666",
	}
}
```