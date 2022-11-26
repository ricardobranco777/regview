package registry

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
)

func dump(resp *http.Response) {
	if !debug || resp == nil {
		return
	}

	dump, err := httputil.DumpRequestOut(resp.Request, true)
	if err != nil {
		log.Print(err)
	} else {
		fmt.Fprintf(os.Stderr, "\n%s", string(dump))
	}

	dump, err = httputil.DumpResponse(resp, true)
	if err != nil {
		log.Print(err)
	} else {
		fmt.Fprintf(os.Stderr, "\n%s\n", string(dump))
	}
}
