// Generates rsrc_windows_amd64.syso (icon #1, manifest, version info) for LeoLink.
// Usage (from the repository root): cd tools/mkres && go run . icon256.png ../../rsrc_windows_amd64.syso 1.11.0
package main

import (
	"image/png"
	"log"
	"os"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

func main() {
	src, out, ver := os.Args[1], os.Args[2], os.Args[3]
	f, err := os.Open(src)
	if err != nil {
		log.Fatal(err)
	}
	img, err := png.Decode(f)
	if err != nil {
		log.Fatal(err)
	}
	icon, err := winres.NewIconFromResizedImage(img, []int{256, 64, 48, 32, 24, 16})
	if err != nil {
		log.Fatal(err)
	}
	rs := &winres.ResourceSet{}
	if err := rs.SetIcon(winres.ID(1), icon); err != nil {
		log.Fatal(err)
	}
	rs.SetManifest(winres.AppManifest{
		Identity:            winres.AssemblyIdentity{Name: "LeoLink"},
		Description:         "LeoLink",
		Compatibility:       winres.Win10AndAbove,
		ExecutionLevel:      winres.AsInvoker,
		DPIAwareness:        winres.DPIPerMonitorV2,
		UseCommonControlsV6: true,
	})
	vi := version.Info{}
	vi.SetFileVersion(ver)
	vi.SetProductVersion(ver)
	vi.Set(version.LangDefault, version.ProductName, "LeoLink")
	vi.Set(version.LangDefault, version.FileDescription, "LeoLink – the PC in Home Assistant, wake-up in Leonzilla mode")
	vi.Set(version.LangDefault, version.CompanyName, "Leoš Vilím · leotech.cz")
	vi.Set(version.LangDefault, version.LegalCopyright, "© Leoš Vilím, MIT License")
	vi.Set(version.LangDefault, version.OriginalFilename, "LeoLink.exe")
	rs.SetVersionInfo(vi)
	w, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	defer w.Close()
	if err := rs.WriteObject(w, winres.ArchAMD64); err != nil {
		log.Fatal(err)
	}
}
