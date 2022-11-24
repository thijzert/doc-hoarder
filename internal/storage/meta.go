package storage

import (
	"context"
	"encoding/xml"
	"time"
)

type DocumentMeta struct {
	xml.Name    `xml:"Document"`
	Title       string
	Author      string
	URL         string `xml:",omitEmpty"`
	ContentType string
	IconID      string         `xml:",omitEmpty"`
	Date        time.Time      `xml:",omitEmpty"`
	Status      DocumentStatus `xml:",omitEmpty"`
	CaptureDate time.Time      `xml:",omitEmpty"`
	Permissions struct {
		Owner       string
		Public      bool
		ReadUsers   []string `xml:"ReadUsers>User,omitEmpty"`
		ReadGroups  []string `xml:"ReadGroups>Group",omitEmpty`
		WriteUsers  []string `xml:"WriteUsers>User",omitEmpty`
		WriteGroups []string `xml:"WriteGroups>Group",omitEmpty`
	}
}

type DocumentStatus string

const (
	StatusDraft  DocumentStatus = "draft"
	StatusStatic                = "static"
)

func ReadMeta(ctx context.Context, trns DocTransaction) (DocumentMeta, error) {
	var rv DocumentMeta
	r, err := trns.ReadRootFile(ctx, "meta.xml")
	if err != nil {
		return rv, err
	}
	dec := xml.NewDecoder(r)
	err = dec.Decode(&rv)
	if err != nil {
		return rv, err
	}
	return rv, nil
}
func WriteMeta(ctx context.Context, trns DocTransaction, meta DocumentMeta) error {
	w, err := trns.WriteRootFile(ctx, "meta.xml")
	if err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "\t")
	return enc.Encode(meta)
}
