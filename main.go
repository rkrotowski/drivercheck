package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/StackExchange/wmi"
	"github.com/pkg/browser"
	"github.com/sqweek/dialog"
	"golang.org/x/sys/windows/registry"
)

type driver struct {
	DeviceName    string
	DriverVersion string
}

func getInstalledVersion() driver {
	var dst []driver
	var device driver
	q := "SELECT DeviceName, DriverVersion FROM Win32_PnPSignedDriver WHERE DeviceName LIKE 'NVIDIA GeForce%'"
	err := wmi.Query(q, &dst)
	if err != nil {
		log.Fatal(err)
	}
	if len(dst) == 0 {
		log.Fatal("No NVIDIA GeForce cards found")
	}
	if len(dst) > 1 {
		for _, v := range dst {
			if v.DriverVersion != dst[0].DriverVersion {
				log.Fatal("Driver Version Mismatch: " + v.DriverVersion + " - " + dst[0].DriverVersion)
			}
		}
	}
	device.DeviceName = dst[0].DeviceName
	fv := strings.ReplaceAll(dst[0].DriverVersion, ".", "")
	device.DriverVersion = fv[len(fv)-5:len(fv)-2] + "." + fv[len(fv)-2:len(fv)]
	return device
}

func getWindowsVersion() uint16 {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", registry.QUERY_VALUE)
	if err != nil {
		log.Fatal(err)
	}
	defer k.Close()

	cv, _, err := k.GetStringValue("CurrentVersion")
	if err != nil {
		log.Fatal(err)
	}

	maj, _, err := k.GetIntegerValue("CurrentMajorVersionNumber")
	if err != nil {
		log.Fatal(err)
	}

	var arch uint16 = 0
	if runtime.GOARCH == "amd64" {
		arch = 1
	}

	if maj == 10 {
		return 56 + arch
	}
	if cv == "6.3" {
		return 40 + arch
	}
	if cv == "6.1" {
		return 18 + arch
	}
	log.Fatal("You are running an unsupported Windows version")
	return 0
}

func getDCH() uint16 {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, "SYSTEM\\CurrentControlSet\\Services\\nvlddmkm", registry.QUERY_VALUE)
	if err != nil {
		return 0
	}
	defer k.Close()

	dch, _, err := k.GetIntegerValue("DCHUVen")
	if err != nil || dch == 0 {
		return 0
	}
	return 1
}

func getAvailableVersion(deviceName string) (string, string) {
	var psID, pfID, osID, langID, dchID uint16
	if strings.Contains(deviceName, "M") {
		psID = 99
		pfID = 758
	} else {
		psID = 98
		pfID = 756
	}
	osID = getWindowsVersion()
	langID = 1 //TODO: Implement other languages, currently defaults to en_US
	dchID = getDCH()
	response, err := http.Get(fmt.Sprintf("https://www.nvidia.com/Download/processDriver.aspx?psid=%d&pfid=%d&rpf=1&osid=%d&lid=%d&dtcid=%d&ctk=0", psID, pfID, osID, langID, dchID))
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	gpuURL, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}

	doc, err := goquery.NewDocument(string(gpuURL))
	if err != nil {
		log.Fatal(err)
	}

	var versions, links []string
	doc.Find("td#tdVersion").Each(func(i int, s *goquery.Selection) {
		versions = append(versions, s.Text())
	})
	doc.Find("a#lnkDwnldBtn").Each(func(i int, s *goquery.Selection) {
		url, exists := s.Attr("href")
		if !exists {
			log.Fatal("Element does not exist")
		}
		links = append(links, url)
	})
	version := strings.TrimSpace(versions[0])
	link := "https://www.nvidia.com" + links[0]
	return link, version
}

func main() {
	if runtime.GOOS != "windows" {
		log.Fatal("Error: OS is not Windows")
	}
	device := getInstalledVersion()
	url, version := getAvailableVersion(device.DeviceName)
	if device.DriverVersion == version[:6] {
		dialog.Message("Driver up to date!").Title("DriverCheck").Info()
	} else {
		download := dialog.Message(fmt.Sprintf("Current: %s\nNew: %s\nDo you want to open the download page?", device.DriverVersion, version[:6])).Title("DriverCheck").YesNo()
		if download {
			err := browser.OpenURL(url)
			if err != nil {
				log.Fatal(fmt.Sprintf("Failed to open %s in default browser", url))
			}
		}
	}
}
