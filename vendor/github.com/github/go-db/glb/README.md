# Package glb

### [godocs](https://godoc.githubapp.com/github.com/github/go-db/glb)

Pull configuration and proxy status information from GLB services.

## Description

One of the properties that is helpful to have about servers is weather or not it is actively serving traffic in GLB.  But there are several steps required to get this information, and there are GLB instances in each site which contain the status for backend hosts in that same site only.  Since there can be so many steps needed to get a complete picture of the GLB proxy status this package exists to make that easier.

After creating the status object and calling `Refresh()`, the result will be an object which contains the parsed csv status output from the glb proxy.  Rows are stored without column names, so a helper function is provided to related the column name to the correct row and column index: `FieldAt()`

## Example

```golang
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/github/go-db/glb"
	"github.com/github/sitesapiclient"
)

func main() {
	sites, err := sitesapi.NewClient(nil, sitesapi.Config{
		Password: os.Getenv("SITES_API_PASSWORD"),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Create the status object
	poolName := "mysql1_ro_main"
	datacenter := "ash1"
	status, err := glb.NewStatus("mysql-proxy", datacenter, &sites, "https")
	if err != nil {
		log.Fatal(err)
	}
	// Get the current status
	result, err := status.Refresh()
	if err != nil {
		log.Fatal(err)
	}

	for i := 0; i < result.Rows(); i++ {
		if result.FieldAt("pxname", i) == poolName {
			fmt.Printf("host=%s dc=%s status=%s\n",
				result.FieldAt("svname", i),
				datacenter,
				result.FieldAt("status", i),
			)
		}
	}

}
```
