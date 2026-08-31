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
	ssdpGroup           = "239.255.255.250:1900"
)

type dlnaServer struct { library *lib.Library; name, addr, uuid string }

func newDLNAServer(library *lib.Library) *dlnaServer {
	addr := os.Getenv("HERMIT_MEDIA_SERVER_ADDR"); if addr == "" { addr = dlnaHTTPAddrDefault }
	name := os.Getenv("HERMIT_MEDIA_SERVER_NAME"); if name == "" { name = "Hermit" }
	h := sha1.Sum([]byte(library.Root))
	return &dlnaServer{library: library, name: name, addr: addr, uuid: fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:])}
}

func (d *dlnaServer) handler() http.Handler { mux:=http.NewServeMux(); mux.HandleFunc("/dlna/device.xml",d.device); mux.HandleFunc("/dlna/content_directory_scpd.xml",d.contentSCPD); mux.HandleFunc("/dlna/connection_manager_scpd.xml",d.connectionManagerSCPD); mux.HandleFunc("/dlna/content_directory/control",d.control); mux.HandleFunc("/dlna/connection_manager/control",d.connectionManagerControl); mux.HandleFunc("/media/",d.media); return mux }

func (d *dlnaServer) device(w http.ResponseWriter,_ *http.Request){ httpXML(w,fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?><root xmlns="urn:schemas-upnp-org:device-1-0"><specVersion><major>1</major><minor>0</minor></specVersion><device><deviceType>urn:schemas-upnp-org:device:MediaServer:1</deviceType><friendlyName>%s</friendlyName><manufacturer>Hermit</manufacturer><modelName>Hermit</modelName><UDN>uuid:%s</UDN><serviceList><service><serviceType>urn:schemas-upnp-org:service:ContentDirectory:1</serviceType><serviceId>urn:upnp-org:serviceId:ContentDirectory</serviceId><SCPDURL>/dlna/content_directory_scpd.xml</SCPDURL><controlURL>/dlna/content_directory/control</controlURL><eventSubURL>/dlna/content_directory/event</eventSubURL></service><service><serviceType>urn:schemas-upnp-org:service:ConnectionManager:1</serviceType><serviceId>urn:upnp-org:serviceId:ConnectionManager</serviceId><SCPDURL>/dlna/connection_manager_scpd.xml</SCPDURL><controlURL>/dlna/connection_manager/control</controlURL><eventSubURL>/dlna/connection_manager/event</eventSubURL></service></serviceList></device></root>`,html.EscapeString(d.name),d.uuid)) }
func (d *dlnaServer) contentSCPD(w http.ResponseWriter,_ *http.Request){ httpXML(w,`<?xml version="1.0" encoding="utf-8"?><scpd xmlns="urn:schemas-upnp-org:service-1-0"><specVersion><major>1</major><minor>0</minor></specVersion><actionList><action><name>Browse</name></action><action><name>GetSystemUpdateID</name></action></actionList></scpd>`) }
func (d *dlnaServer) connectionManagerSCPD(w http.ResponseWriter,_ *http.Request){ httpXML(w,`<?xml version="1.0" encoding="utf-8"?><scpd xmlns="urn:schemas-upnp-org:service-1-0"><specVersion><major>1</major><minor>0</minor></specVersion><actionList><action><name>GetProtocolInfo</name></action></actionList></scpd>`) }

func (d *dlnaServer) control(w http.ResponseWriter,r *http.Request){ var env soapEnvelope; if err:=xml.NewDecoder(r.Body).Decode(&env); err!=nil {soapFault(w,"InvalidArgs");return}; action:=actionName(r.Header.Get("SOAPAction"),env.Body.Action); switch action { case "Browse": var in browseInput; if err:=xml.Unmarshal(env.Body.Action,&in);err!=nil{soapFault(w,"InvalidArgs");return}; result,n,total:=d.browse(r.Context(),in.ObjectID,in.BrowseFlag,in.StartingIndex,in.RequestedCount); soapBrowse(w,result,n,total); case "GetSystemUpdateID": soapActionResponse(w,"GetSystemUpdateIDResponse",`<Id>1</Id>`); default: soapFault(w,"InvalidAction") } }
func (d *dlnaServer) connectionManagerControl(w http.ResponseWriter,_ *http.Request){ soapActionResponse(w,"GetProtocolInfoResponse",`<Source>http-get:*:video/mp4:*</Source><Sink></Sink>`) }

func (d *dlnaServer) browse(ctx context.Context, objectID, flag string, start, requested uint32)(string,uint32,uint32){ _=ctx; currentRel:=""; root:=d.library.Root; if objectID!="0"&&objectID!=""{rel,err:=url.PathUnescape(objectID);if err!=nil{return didlList(""),0,0};currentRel=filepath.ToSlash(strings.TrimPrefix(rel,"/"));root=filepath.Join(d.library.Root,filepath.FromSlash(currentRel))}; items,err:=os.ReadDir(root);if err!=nil{return didlList(""),0,0}; entries:=make([]didlObject,0,len(items)); for _,e:=range items{if strings.HasPrefix(e.Name(),"."){continue};full:=filepath.Join(root,e.Name());if e.IsDir(){rel:=filepath.ToSlash(filepath.Join(currentRel,e.Name()));entries=append(entries,didlObject{ID:url.PathEscape(rel),Parent:objectID,Title:e.Name(),Container:true});continue};if !isMediaExt(e.Name()){continue};rel,err:=filepath.Rel(d.library.Root,full);if err!=nil{continue};entries=append(entries,didlObject{ID:url.PathEscape(filepath.ToSlash(rel)),Parent:objectID,Title:e.Name(),Size:fileSize(full)})};sort.Slice(entries,func(i,j int)bool{return strings.ToLower(entries[i].Title)<strings.ToLower(entries[j].Title)});if flag=="BrowseMetadata"{if objectID=="0"||objectID==""{return didlList(didlContainer(d.name,"0","-1")),1,1};for _,x:=range entries{if x.ID==objectID{return didlList(x.toDIDL(d.baseURL())),1,1}};return didlList(""),0,0};if start>=uint32(len(entries)){return didlList(""),0,uint32(len(entries))};end:=uint32(len(entries));if requested!=0&&start+requested<end{end=start+requested};var b strings.Builder;for _,x:=range entries[start:end]{b.WriteString(x.toDIDL(d.baseURL()))};return didlList(b.String()),uint32(end-start),uint32(len(entries)) }

func (d *dlnaServer) media(w http.ResponseWriter,r *http.Request){rel,err:=url.PathUnescape(strings.TrimPrefix(r.URL.Path,"/media/"));if err!=nil{http.NotFound(w,r);return};clean:=filepath.Clean(filepath.FromSlash(rel));if clean=="."||clean==".."||strings.HasPrefix(clean,".."+string(filepath.Separator)){http.NotFound(w,r);return};full:=filepath.Join(d.library.Root,clean);root,_:=filepath.Abs(d.library.Root);abs,_:=filepath.Abs(full);if abs!=root&&!strings.HasPrefix(abs,root+string(filepath.Separator)){http.NotFound(w,r);return};f,err:=os.Open(full);if err!=nil{http.NotFound(w,r);return};defer f.Close();info,err:=f.Stat();if err!=nil||info.IsDir(){http.NotFound(w,r);return};w.Header().Set("Accept-Ranges","bytes");w.Header().Set("Content-Type",mimeType(info.Name()));http.ServeContent(w,r,info.Name(),info.ModTime(),f)}

func(d *dlnaServer)start(ctx context.Context,_ func(string)){ln,err:=net.Listen("tcp",d.addr);if err!=nil{return};srv:=&http.Server{Handler:d.handler(),ReadHeaderTimeout:5*time.Second};go func(){<-ctx.Done();c,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel();_=srv.Shutdown(c)}();go func(){_=srv.Serve(ln)}();go d.ssdp(ctx,d.baseURLForListener(ln))}
func(d *dlnaServer)baseURL()string{return "http://"+localIPv4()+":8789"}
func(d *dlnaServer)baseURLForListener(ln net.Listener)string{port:="8789";if a,ok:=ln.Addr().(*net.TCPAddr);ok{port=fmt.Sprintf("%d",a.Port)};return "http://"+localIPv4()+":"+port}
func localIPv4()string{if c,err:=net.Dial("udp4","8.8.8.8:80");err==nil{defer c.Close();if a,ok:=c.LocalAddr().(*net.UDPAddr);ok{return a.IP.String()}};ifs,_:=net.Interfaces();for _,in:=range ifs{if in.Flags&net.FlagUp==0||in.Flags&net.FlagLoopback!=0{continue};addrs,_:=in.Addrs();for _,a:=range addrs{if n,ok:=a.(*net.IPNet);ok&&n.IP.To4()!=nil{return n.IP.To4().String()}}};return "127.0.0.1"}

func(d *dlnaServer)ssdp(ctx context.Context,base string){g:=net.ParseIP("239.255.255.250");conn,err:=net.ListenMulticastUDP("udp4",nil,&net.UDPAddr{IP:g,Port:1900});if err!=nil{return};defer conn.Close();usn:="uuid:"+d.uuid+"::urn:schemas-upnp-org:device:MediaServer:1";loc:=base+"/dlna/device.xml";announce:=func(){msg:=fmt.Sprintf("NOTIFY * HTTP/1.1\r\nHOST: %s\r\nCACHE-CONTROL: max-age=1800\r\nLOCATION: %s\r\nNT: urn:schemas-upnp-org:device:MediaServer:1\r\nNTS: ssdp:alive\r\nUSN: %s\r\n\r\n",ssdpGroup,loc,usn);_,_=conn.WriteToUDP([]byte(msg),&net.UDPAddr{IP:g,Port:1900})};announce();buf:=make([]byte,4096);tick:=time.NewTicker(60*time.Second);defer tick.Stop();for{_=conn.SetReadDeadline(time.Now().Add(time.Second));n,src,err:=conn.ReadFromUDP(buf);if err==nil&&strings.Contains(strings.ToLower(string(buf[:n])),"m-search"){resp:=fmt.Sprintf("HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=1800\r\nDATE: %s\r\nEXT:\r\nLOCATION: %s\r\nSERVER: Hermit/1.0 UPnP/1.0\r\nST: urn:schemas-upnp-org:device:MediaServer:1\r\nUSN: %s\r\n\r\n",time.Now().UTC().Format(http.TimeFormat),loc,usn);_,_=conn.WriteToUDP([]byte(resp),src)};select{case<-ctx.Done():return;case<-tick.C:announce();default:}}}

func httpXML(w http.ResponseWriter,body string){w.Header().Set("Content-Type","text/xml; charset=utf-8");w.WriteHeader(http.StatusOK);_,_=w.Write([]byte(body))}
type didlObject struct{ID,Parent,Title string;Size int64;Container bool}
func(x didlObject)toDIDL(base string)string{if x.Container{return didlContainer(x.Title,x.ID,x.Parent)};return fmt.Sprintf(`<item id="%s" parentID="%s" restricted="1"><dc:title xmlns:dc="http://purl.org/dc/elements/1.1/">%s</dc:title><upnp:class xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">object.item.videoItem</upnp:class><res protocolInfo="http-get:*:%s:*" size="%d">%s/media/%s</res></item>`,html.EscapeString(x.ID),html.EscapeString(x.Parent),html.EscapeString(x.Title),mimeType(x.Title),x.Size,strings.TrimRight(base,"/"),x.ID)}
func didlContainer(title,id,parent string)string{return fmt.Sprintf(`<container id="%s" parentID="%s" restricted="1"><dc:title xmlns:dc="http://purl.org/dc/elements/1.1/">%s</dc:title><upnp:class xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">object.container.storageFolder</upnp:class></container>`,html.EscapeString(id),html.EscapeString(parent),html.EscapeString(title))}
func didlList(content string)string{return `<?xml version="1.0" encoding="utf-8"?><DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">`+content+`</DIDL-Lite>`}
func mimeType(name string)string{switch strings.ToLower(filepath.Ext(name)){case ".mp4", ".m4v":return "video/mp4";case ".mkv":return "video/x-matroska";case ".webm":return "video/webm";case ".ts":return "video/mp2t";default:return "application/octet-stream"}}
func isMediaExt(name string)bool{switch strings.ToLower(filepath.Ext(name)){case ".mp4", ".m4v", ".mkv", ".webm", ".ts":return true;default:return false}}
func fileSize(path string)int64{if s,err:=os.Stat(path);err==nil{return s.Size()};return 0}
type soapEnvelope struct{Body struct{Action xml.RawMessage `xml:",any"`} `xml:"Body"`}
type browseInput struct{ObjectID string `xml:"ObjectID"`;BrowseFlag string `xml:"BrowseFlag"`;StartingIndex uint32 `xml:"StartingIndex"`;RequestedCount uint32 `xml:"RequestedCount"`}
func actionName(header string,raw xml.RawMessage)string{if i:=strings.Index(header,"#");i>=0{return strings.Trim(header[i+1:],`"'`)};s:=string(raw);a:=strings.Index(s,"<");if a<0{return ""};s=s[a+1:];b:=strings.IndexAny(s," >");if b<0{return ""};n:=s[:b];if c:=strings.LastIndex(n,":");c>=0{n=n[c+1:]};return n}
func soapBrowse(w http.ResponseWriter,result string,returned,total uint32){soapActionResponse(w,"BrowseResponse",fmt.Sprintf(`<Result>%s</Result><NumberReturned>%d</NumberReturned><TotalMatches>%d</TotalMatches><UpdateID>1</UpdateID>`,xmlEscapeText(result),returned,total))}
func soapActionResponse(w http.ResponseWriter,action,body string){w.Header().Set("Content-Type",`text/xml; charset="utf-8"`);_,_=fmt.Fprintf(w,`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:%s xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">%s</u:%s></s:Body></s:Envelope>`,action,body,action)}
func soapFault(w http.ResponseWriter,code string){w.Header().Set("Content-Type",`text/xml; charset="utf-8"`);w.WriteHeader(http.StatusInternalServerError);_,_=fmt.Fprintf(w,`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>%s</faultstring></s:Fault></s:Body></s:Envelope>`,html.EscapeString(code))}
func xmlEscapeText(s string)string{var b strings.Builder;_=xml.EscapeText(&b,[]byte(s));return b.String()}
