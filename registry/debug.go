package registry

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
)

func dump(it interface{}) {
	var dump []byte
	var err error

	if !debug {
		return
	}

	switch data := it.(type) {
	case *http.Request:
		if data == nil {
			return
		}
		dump, err = httputil.DumpRequestOut(data, true)
	case *http.Response:
		if data == nil {
			return
		}
		dump, err = httputil.DumpResponse(data, true)
	}
	if err != nil {
		log.Print(err)
	}

	fmt.Fprintf(os.Stderr, "\n%s\n", string(dump))
}
