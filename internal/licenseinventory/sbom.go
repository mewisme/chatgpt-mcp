package licenseinventory

import (
	"fmt"
)

const spdxCreated = "2026-01-01T00:00:00Z"

type spdxDocument struct {
	SPDXVersion       string             `json:"spdxVersion"`
	DataLicense       string             `json:"dataLicense"`
	SPDXID            string             `json:"SPDXID"`
	Name              string             `json:"name"`
	DocumentNamespace string             `json:"documentNamespace"`
	CreationInfo      spdxCreationInfo   `json:"creationInfo"`
	Packages          []spdxPackage      `json:"packages"`
	Relationships     []spdxRelationship `json:"relationships"`
}

type spdxCreationInfo struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
}

type spdxPackage struct {
	Name             string `json:"name"`
	SPDXID           string `json:"SPDXID"`
	VersionInfo      string `json:"versionInfo"`
	DownloadLocation string `json:"downloadLocation"`
	FilesAnalyzed    bool   `json:"filesAnalyzed"`
	LicenseConcluded string `json:"licenseConcluded"`
	LicenseDeclared  string `json:"licenseDeclared"`
	CopyrightText    string `json:"copyrightText"`
}

type spdxRelationship struct {
	SPDXElementID      string `json:"spdxElementId"`
	RelationshipType   string `json:"relationshipType"`
	RelatedSPDXElement string `json:"relatedSpdxElement"`
}

func renderSBOM(inv Inventory) spdxDocument {
	doc := spdxDocument{
		SPDXVersion:       "SPDX-2.3",
		DataLicense:       "CC0-1.0",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              "chatgpt-mcp-" + inv.Artifact,
		DocumentNamespace: "https://github.com/mewisme/chatgpt-mcp/spdx/" + inv.Artifact,
		CreationInfo:      spdxCreationInfo{Created: spdxCreated, Creators: []string{"Tool: chatgpt-mcp-license-inventory"}},
	}
	for i, pkg := range inv.Packages {
		id := fmt.Sprintf("SPDXRef-Package-%d", i)
		location := pkg.Source
		if location == "" {
			location = "NOASSERTION"
		}
		doc.Packages = append(doc.Packages, spdxPackage{
			Name: pkg.Name, SPDXID: id, VersionInfo: pkg.Version, DownloadLocation: location,
			LicenseConcluded: pkg.License, LicenseDeclared: pkg.License, CopyrightText: "NOASSERTION",
		})
		rel := "DESCRIBES"
		if i > 0 {
			rel = "DEPENDS_ON"
		}
		from := "SPDXRef-DOCUMENT"
		if i > 0 {
			from = "SPDXRef-Package-0"
		}
		doc.Relationships = append(doc.Relationships, spdxRelationship{SPDXElementID: from, RelationshipType: rel, RelatedSPDXElement: id})
	}
	return doc
}
