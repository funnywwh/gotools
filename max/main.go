package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
)

var gmax float64
var regd = regexp.MustCompile(`[.\d]+`)

func main() {
	var err error
	var tmp float64
	var fmtstr string = `\d+`
	if len(os.Args) > 1 {
		fmtstr = os.Args[1]
	}
	re := regexp.MustCompile(fmtstr)
	fmt.Println(fmtstr)
	scaner := bufio.NewScanner(os.Stdin)
	scaner.Split(bufio.ScanLines)
	for scaner.Scan() {
		tmp = 0
		line := scaner.Text()
		m := re.FindAllStringSubmatch(line, 1)
		if len(m) > 0 {
			fmt.Printf("match:%v\n", m)
			if len(m[0]) > 1 {
				tmp, _ = strconv.ParseFloat(m[0][1], 64)
				if float64(tmp) > gmax {
					gmax = tmp
					fmt.Printf("max[%f]\n", gmax)
				}

			}

		}

		if eof, ok := err.(error); ok && eof == io.EOF {
			return
		}
	}
}
