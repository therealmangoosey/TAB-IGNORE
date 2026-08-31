package srv

import (
	"context"
	"crypto/sha1"
	"encoding/xml"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/internal/lib"
)

const (
	dlnaHTTPAddrDefault = "0.0.0.0:8789"
	ssdpAddr            = "239.255.255.250:1900"
)

type dlnaServer struct {
	library *lib.Library
	name    string
	addr    string
	uuid    string
}

func newDLNAServer(library *lib.Library) *dlnaServer {
	addr := os.Getenv("HERMIT_MEDIA_SERVER_ADDR")
	if addr == "" {
		addr = dlnaHTTPAddrDefault
	}
	name := os.Getenv("HERMIT_MEDIA_SERVER_NAME")
	if name == "" {
		name = "Hermit"
	}
	h := sha1.Sum([]byte(library.Root))
	return &dlnaServer{library: library, name: name, addr: addr, uuid: fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:])}
}

func (d *dlnaServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/dlna/device.xml", d.device)
	mux.HandleFunc("/dlna/content_directory.xml", d.serviceDescription)
	mux.HandleFunc("/dlna/content_directory_scpd.xml", d.contentSCPD)
	mux.HandleFunc("/dlna/connection_manager.xml", d.connectionManager)
	mux.HandleFunc("/dlna/connection_manager_scpd.xml", d.connectionManagerSCPD)
	mux.HandleFunc("/dlna/content_directory/control", d.control)
	mux.HandleFunc("/dlna/connection_manager/control", d.connectionManagerControl)
	mux.HandleFunc("/media/", d.media)
	return mux
}

func (d *dlnaServer) device(w http.ResponseWriter, r *http.Request) {
	base := d.baseURL()
	xmlText := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0"><specVersion><major>1</major><minor>0</minor></specVersion><device><deviceType>urn:schemas-upnp-org:device:MediaServer:1</deviceType><friendlyName>%s</friendlyName><manufacturer>Hermit</manufacturer><modelName>Hermit</modelName><UDN>uuid:%s</UDN><serviceList><service><serviceType>urn:schemas-upnp-org:service:ContentDirectory:1</serviceType><serviceId>urn:upnp-org:serviceId:ContentDirectory</serviceId><SCPDURL>/dlna/content_directory_scpd.xml</SCPDURL><controlURL>/dlna/content_directory/control</controlURL><eventSubURL>/dlna/content_directory/event</eventSubURL></service><service><serviceType>urn:schemas-upnp-org:service:ConnectionManager:1</serviceType><serviceId>urn:upnp-org:serviceId:ConnectionManager</serviceId><SCPDURL>/dlna/connection_manager_scpd.xml</SCPDURL><controlURL>/dlna/connection_manager/control</controlURL><eventSubURL>/dlna/connection_manager/event</eventSubURL></service></serviceList></device></root>`, html.EscapeString(d.name), d.uuid)
	httpXML(w, xmlText)
	_ = base
}

func (d *dlnaServer) serviceDescription(w http.ResponseWriter, r *http.Request) {
	base := d.baseURL()
	text := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?><scpd xmlns="urn:schemas-upnp-org:service-1-0"><specVersion><major>1</major><minor>0</minor></specVersion><serviceStateTable><stateVariable sendEvents="yes"><name>SystemUpdateID</name><dataType>ui4</dataType></stateVariable><stateVariable sendEvents="no"><name>A_ARG_TYPE_BrowseFlag</name><dataType>string</dataType><allowedValueList><allowedValue>BrowseMetadata</allowedValue><allowedValue>BrowseDirectChildren</allowedValue></allowedValueList></stateVariable><stateVariable sendEvents="no"><name>A_ARG_TYPE_ObjectID</name><dataType>string</dataType></stateVariable><stateVariable sendEvents="no"><name>A_ARG_TYPE_Result</name><dataType>string</dataType></stateVariable><stateVariable sendEvents="no"><name>A_ARG_TYPE_Count</name><dataType>ui4</dataType></stateVariable></serviceStateTable><actionList><action><name>Browse</name><argumentList><argument><name>ObjectID</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_ObjectID</relatedStateVariable></argument><argument><name>BrowseFlag</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_BrowseFlag</relatedStateVariable></argument><argument><name>Filter</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_Result</relatedStateVariable></argument><argument><name>StartingIndex</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable></argument><argument><name>RequestedCount</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable></argument><argument><name>SortCriteria</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_Result</relatedStateVariable></argument><argument><name>Result</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Result</relatedStateVariable></argument><argument><name>NumberReturned</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable></argument><argument><name>TotalMatches</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable></argument><argument><name>UpdateID</name><direction>out</direction><relatedStateVariable>SystemUpdateID</relatedStateVariable></argument></argumentList></action><action><name>GetSystemUpdateID</name><argumentList><argument><name>Id</name><direction>out</direction><relatedStateVariable>SystemUpdateID</relatedStateVariable></argument></argumentList></action></actionList></scpd>`, base)
	httpXML(w, text)
}

func (d *dlnaServer) connectionManager(w http.ResponseWriter, r *http.Request) {
	httpXML(w, `<?xml version="1.0" encoding="utf-8"?><scpd xmlns="urn:schemas-upnp-org:service-1-0"><specVersion><major>1</major><minor>0</minor></specVersion></scpd>`)
}

func (d *dlnaServer) contentSCPD(w http.ResponseWriter, r *http.Request) {
	httpXML(w, `<?xml version="1.0" encoding="utf-8"?><scpd xmlns="urn:schemas-upnp-org:service-1-0"><specVersion><major>1</major><minor>0</minor></specVersion><actionList><action><name>Browse</name></action><action><name>GetSystemUpdateID</name></action></actionList></scpd>`)
}

func (d *dlnaServer) connectionManagerSCPD(w http.ResponseWriter, r *http.Request) {
	httpXML(w, `<?xml version="1.0" encoding="utf-8"?><scpd xmlns="urn:schemas-upnp-org:service-1-0"><specVersion><major>1</major><minor>0</minor></specVersion><actionList><action><name>GetProtocolInfo</name></action></actionList></scpd>`)
}

func (d *dlnaServer) control(w http.ResponseWriter, r *http.Request) {
	var envelope soapEnvelope
	if err := xml.NewDecoder(r.Body).Decode(&envelope); err != nil {
		soapFault(w, "InvalidArgs")
		return
	}
	action := actionName(r.Header.Get("SOAPAction"), envelope.Body.Action)
	switch action {
	case "Browse":
		var in browseInput
		if err := decodeAction(envelope.Body.Action, &in); err != nil {
			soapFault(w, "InvalidArgs")
			return
		}
		result, returned, total := d.browse(r.Context(), in.ObjectID, in.BrowseFlag, in.StartingIndex, in.RequestedCount)
		soapBrowse(w, result, returned, total)
	case "GetSystemUpdateID":
		soapActionResponse(w, "GetSystemUpdateIDResponse", `<Id>1</Id>`)
	default:
		soapFault(w, "InvalidAction")
	}
}

func (d *dlnaServer) connectionManagerControl(w http.ResponseWriter, r *http.Request) {
	soapActionResponse(w, "GetProtocolInfoResponse", `<Source>http-get:*:video/mp4:*,http-get:*:video/x-matroska:*,http-get:*:video/webm:*,http-get:*:video/mp2t:*</Source><Sink></Sink>`)
}

func (d *dlnaServer) browse(ctx context.Context, objectID, flag string, start, requested uint32) (string, uint32, uint32) {
	entries := []didlObject{}
	root := d.library.Root
	if objectID != "0" {
		rel, err := url.PathUnescape(objectID)
		if err != nil {
			return "", 0, 0
		}
		root = filepath.Join(d.library.Root, filepath.FromSlash(strings.TrimPrefix(rel, "/")))
	}
	items, err := os.ReadDir(root)
	if err != nil {
		return "", 0, 0
	}
	for _, e := range items {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(root, e.Name())
		if e.IsDir() {
			id := objectID
			if id == "" { id = "0" }
			if id == "0" { id = "" }
			rel := filepath.ToSlash(filepath.Join(strings.Trim(id, "/"), e.Name()))
			entries = append(entries, didlObject{ID: url.PathEscape(rel), Parent: objectID, Title: e.Name(), Container: true})
			continue
		}
		if !isMediaExt(e.Name()) {
			continue
		}
		rel, err := filepath.Rel(d.library.Root, full)
		if err != nil { continue }
		entries = append(entries, didlObject{ID: url.PathEscape(filepath.ToSlash(rel)), Parent: objectID, Title: e.Name(), Path: filepath.ToSlash(rel), Size: fileSize(full), Container: false})
	}
	sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].Title) < strings.ToLower(entries[j].Title) })
	if flag == "BrowseMetadata" {
		if objectID == "0" { return didlContainer(d.name, "0", "-1"), 1, 1 }
		for _, x := range entries { if x.ID == objectID { return x.toDIDL(d.baseURL()), 1, 1 } }
		return "", 0, 0
	}
	if start >= uint32(len(entries)) { return didlContainerList(nil), 0, uint32(len(entries)) }
	end := uint32(len(entries))
	if requested != 0 && start+requested < end { end = start+requested }
	selected := entries[start:end]
	var b strings.Builder
	for _, x := range selected { b.WriteString(x.toDIDL(d.baseURL())) }
	return didlContainerListRaw(b.String()), uint32(len(selected)), uint32(len(entries))
}

func (d *dlnaServer) media(w http.ResponseWriter, r *http.Request) {
	rel, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/media/"))
	if err != nil { http.NotFound(w, r); return }
	full := filepath.Join(d.library.Root, filepath.FromSlash(filepath.Clean(rel)))
	root, _ := filepath.Abs(d.library.Root); abs, _ := filepath.Abs(full)
	if root != abs && !strings.HasPrefix(abs, root+string(filepath.Separator)) { http.NotFound(w, r); return }
	f, err := os.Open(full); if err != nil { http.NotFound(w, r); return }
	defer f.Close()
	info, err := f.Stat(); if err != nil || info.IsDir() { http.NotFound(w, r); return }
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", mimeType(info.Name()))
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func (d *dlnaServer) start(ctx context.Context, onReady func(string)) {
	ln, err := net.Listen("tcp", d.addr); if err != nil { return }
	srv := &http.Server{Handler: d.handler(), ReadHeaderTimeout: 5 * time.Second}
	onReady(d.baseURLForListener(ln))
	go func() { <-ctx.Done(); c, cancel := context.WithTimeout(context.Background(), 3*time.Second); defer cancel(); _ = srv.Shutdown(c) }()
	go func() { _ = srv.Serve(ln) }()
	go d.ssdp(ctx, d.baseURLForListener(ln))
}

func (d *dlnaServer) baseURL() string { return "http://127.0.0.1:8789" }
func (d *dlnaServer) baseURLForListener(ln net.Listener) string {
	port := "8789"; if a, ok := ln.Addr().(*net.TCPAddr); ok { port = fmt.Sprintf("%d", a.Port) }
	ip := localIPv4(); return "http://" + ip + ":" + port
}
func localIPv4() string {
	if c, err := net.Dial("udp4", "8.8.8.8:80"); err == nil { defer c.Close(); if a, ok := c.LocalAddr().(*net.UDPAddr); ok { return a.IP.String() } }
	ifs, _ := net.Interfaces(); for _, in := range ifs { if in.Flags&net.FlagUp == 0 || in.Flags&net.FlagLoopback != 0 { continue }; addrs, _ := in.Addrs(); for _, a := range addrs { if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil { return ipnet.IP.To4().String() } } }
	return "127.0.0.1"
}

func (d *dlnaServer) ssdp(ctx context.Context, base string) {
	conn, err := net.ListenPacket("udp4", ":0"); if err != nil { return }
	defer conn.Close()
	group := &net.UDPAddr{IP: net.ParseIP(ssdpAddr[:strings.Index(ssdpAddr, ":")]), Port: 1900}
	usn := "uuid:" + d.uuid + "::urn:schemas-upnp-org:device:MediaServer:1"
	loc := base + "/dlna/device.xml"
	announce := func(dst *net.UDPAddr) { msg := fmt.Sprintf("NOTIFY * HTTP/1.1\r\nHOST: %s\r\nCACHE-CONTROL: max-age=1800\r\nLOCATION: %s\r\nNT: urn:schemas-upnp-org:device:MediaServer:1\r\nNTS: ssdp:alive\r\nUSN: %s\r\n\r\n", ssdpAddr, loc, usn); _, _ = conn.WriteTo([]byte(msg), dst) }
	search := make([]byte, 4096)
	t := time.NewTicker(30 * time.Second); defer t.Stop()
	announce(group)
	for { conn.SetReadDeadline(time.Now().Add(1*time.Second)); n, src, err := conn.ReadFromUDP(search); if err == nil && strings.Contains(strings.ToLower(string(search[:n])), "m-search") { resp := fmt.Sprintf("HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=1800\r\nDATE: %s\r\nEXT:\r\nLOCATION: %s\r\nSERVER: Hermit/1.0 UPnP/1.0\r\nST: urn:schemas-upnp-org:device:MediaServer:1\r\nUSN: %s\r\n\r\n", time.Now().UTC().Format(http.TimeFormat), loc, usn); _, _ = conn.WriteToUDP([]byte(resp), src) }; select { case <-ctx.Done(): return; case <-t.C: announce(group); default: } }
}

func httpXML(w http.ResponseWriter, body string) { w.Header().Set("Content-Type", "text/xml; charset=utf-8"); w.WriteHeader(http.StatusOK); _, _ = w.Write([]byte(body)) }
func (d *dlnaServer) baseFor(id string) string { return d.baseURL() + "/media/" + id }

type didlObject struct { ID, Parent, Title, Path string; Size int64; Container bool }
func (x didlObject) toDIDL(base string) string { if x.Container { return didlContainer(x.Title, x.ID, x.Parent) }; u := base + "/media/" + x.ID; return fmt.Sprintf(`<item id="%s" parentID="%s" restricted="1"><dc:title xmlns:dc="http://purl.org/dc/elements/1.1/">%s</dc:title><upnp:class xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">object.item.videoItem</upnp:class><res protocolInfo="http-get:*:%s:*" size="%d">%s</res></item>`, html.EscapeString(x.ID), html.EscapeString(x.Parent), html.EscapeString(x.Title), mimeType(x.Title), x.Size, html.EscapeString(u)) }
func didlContainer(title, id, parent string) string { return fmt.Sprintf(`<container id="%s" parentID="%s" restricted="1" searchable="0"><dc:title xmlns:dc="http://purl.org/dc/elements/1.1/">%s</dc:title><upnp:class xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">object.container.storageFolder</upnp:class></container>`, html.EscapeString(id), html.EscapeString(parent), html.EscapeString(title)) }
func didlContainerListRaw(content string) string { return `<?xml version="1.0" encoding="utf-8"?><DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">`+content+`</DIDL-Lite>` }
func didlContainerList(v []didlObject) string { var b strings.Builder; for _, x := range v { b.WriteString(x.toDIDL("")) }; return didlContainerListRaw(b.String()) }
func mimeType(name string) string { switch strings.ToLower(filepath.Ext(name)) { case ".mp4", ".m4v": return "video/mp4"; case ".mkv": return "video/x-matroska"; case ".webm": return "video/webm"; case ".ts": return "video/mp2t"; default: return "application/octet-stream" } }
func isMediaExt(name string) bool { switch strings.ToLower(filepath.Ext(name)) { case ".mp4", ".mkv", ".m4v", ".webm", ".ts": return true; default: return false } }
func fileSize(path string) int64 { if st, err := os.Stat(path); err == nil { return st.Size() }; return 0 }

type soapEnvelope struct { Body struct { Action xml.RawMessage `xml:",any"` } `xml:"Body"` }
type browseInput struct { ObjectID string `xml:"ObjectID"`; BrowseFlag string `xml:"BrowseFlag"`; StartingIndex uint32 `xml:"StartingIndex"`; RequestedCount uint32 `xml:"RequestedCount"` }
func decodeAction(raw xml.RawMessage, out any) error { return xml.Unmarshal(raw, out) }
func actionName(header string, raw xml.RawMessage) string { if i := strings.Index(header, "#"); i >= 0 { return strings.Trim(header[i+1:], `"'`) }; s := string(raw); a := strings.Index(s, ":"); b := strings.Index(s[a+1:], ">"); if a >= 0 && b >= 0 { return strings.Trim(s[a+1:a+1+b], ` <>")` }; return "" }
func soapBrowse(w http.ResponseWriter, result string, returned, total uint32) { soapActionResponse(w, "BrowseResponse", fmt.Sprintf(`<Result>%s</Result><NumberReturned>%d</NumberReturned><TotalMatches>%d</TotalMatches><UpdateID>1</UpdateID>`, xmlEscapeText(result), returned, total)) }
func soapActionResponse(w http.ResponseWriter, action string, body string) { w.Header().Set("Content-Type", `text/xml; charset="utf-8"`); _, _ = fmt.Fprintf(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:%s xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">%s</u:%s></s:Body></s:Envelope>`, action, body, action) }
func soapFault(w http.ResponseWriter, code string) { w.Header().Set("Content-Type", `text/xml; charset="utf-8"`); w.WriteHeader(http.StatusInternalServerError); _, _ = fmt.Fprintf(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>%s</faultstring></s:Fault></s:Body></s:Envelope>`, html.EscapeString(code)) }
func xmlEscapeText(s string) string { var b strings.Builder; _ = xml.EscapeText(&b, []byte(s)); return b.String() }

// Keep the compiler honest when some XML types are only used by reflection.
var _ = []byte{}
