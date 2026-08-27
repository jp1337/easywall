package shared

// The service catalogue: a name an operator recognises, the ports it listens on,
// and a suggested source restriction.
//
// A Go slice rather than an embedded file. The tree has exactly one go:embed —
// the commented TOML defaults, which exist to be written back out — and a table
// the compiler checks is worth more here than a file somebody can ship broken.
//
// Nothing in this file decides a firewall rule. A stored rule keeps its own port
// and its own sources; Service is a label pointing back here for display. So an
// entry may be corrected, and an id it no longer knows renders as plain text
// beside a rule that has not changed.

// Suggestion is what the catalogue recommends about who should reach a service.
// Two constants, not per-entry CIDR literals: the reasoning is a sentence that
// has to be translated, and the private ranges belong in one place.
type Suggestion string

const (
	// SuggestPrivate: a service with no business being on the public internet.
	SuggestPrivate Suggestion = "private"
	// SuggestAnywhere: a service whose whole point is to be reachable.
	SuggestAnywhere Suggestion = "anywhere"
)

// AllSuggestions is the complete list, and it is what the locale guard hangs
// off: both strict locales must carry a rationale for each.
var AllSuggestions = []Suggestion{SuggestPrivate, SuggestAnywhere}

// PrivateRanges is what "private" means when the picker fills the field.
//
// RFC 1918 plus fc00::/7. RFC 1918 alone would be an IPv4 answer to a question a
// dual-stack host asks in both families: an operator on an IPv6 LAN who accepted
// the suggestion would have restricted the port to networks their own address is
// not in, and found out by being locked out of it. A LAN numbered out of global
// IPv6 space is still not covered — no constant can cover it, which is why the
// picker writes these into a visible, editable field and the aside beside it
// says what "private" means rather than the interface deciding quietly.
var PrivateRanges = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"fc00::/7",
}

// SuggestedSources is the source list a suggestion fills in. Anywhere is the
// empty list, which is what an unrestricted rule has always been.
func SuggestedSources(s Suggestion) []string {
	if s == SuggestPrivate {
		return append([]string(nil), PrivateRanges...)
	}
	return nil
}

// ServicePort is one port a service listens on. Description is not translated:
// "DNS", "HTTPS" and "Web interface" are the same words in the languages this
// product ships, and a translation file is where a service's port list would go
// stale unnoticed.
type ServicePort struct {
	Proto       string // "tcp" or "udp"
	Port        string // the stored form: "80" or "8000:9000"
	Description string
}

// Service is one catalogue entry.
type Service struct {
	ID      string // stable, never reused — a stored rule points at it
	Name    string // a proper noun, never translated
	Ports   []ServicePort
	Suggest Suggestion
}

// Catalogue is every entry, ordered by name so the picker needs no sort.
var Catalogue = []Service{
	{ID: "adguardhome", Name: "AdGuard Home", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "3000", Description: "Web interface"},
		{Proto: "tcp", Port: "53", Description: "DNS"},
		{Proto: "udp", Port: "53", Description: "DNS"},
	}},
	{ID: "gitea", Name: "Gitea", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "3000", Description: "Web interface"},
	}},
	{ID: "grafana", Name: "Grafana", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "3000", Description: "Web interface"},
	}},
	{ID: "homeassistant", Name: "Home Assistant", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "8123", Description: "Web interface"},
	}},
	{ID: "http", Name: "HTTP", Suggest: SuggestAnywhere, Ports: []ServicePort{
		{Proto: "tcp", Port: "80", Description: "HTTP"},
	}},
	{ID: "https", Name: "HTTPS", Suggest: SuggestAnywhere, Ports: []ServicePort{
		{Proto: "tcp", Port: "443", Description: "HTTPS"},
	}},
	{ID: "immich", Name: "Immich", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "2283", Description: "Web interface"},
	}},
	{ID: "jellyfin", Name: "Jellyfin", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "8096", Description: "Web interface"},
	}},
	{ID: "minecraft", Name: "Minecraft", Suggest: SuggestAnywhere, Ports: []ServicePort{
		{Proto: "tcp", Port: "25565", Description: "Java Edition"},
	}},
	{ID: "minio", Name: "MinIO", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "9000", Description: "S3 API"},
		{Proto: "tcp", Port: "9001", Description: "Console"},
	}},
	{ID: "mosquitto", Name: "Mosquitto (MQTT)", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "1883", Description: "MQTT"},
		{Proto: "tcp", Port: "8883", Description: "MQTT over TLS"},
	}},
	{ID: "mysql", Name: "MySQL / MariaDB", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "3306", Description: "Database"},
	}},
	{ID: "nfs", Name: "NFS", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "2049", Description: "NFSv4"},
	}},
	{ID: "openvpn", Name: "OpenVPN", Suggest: SuggestAnywhere, Ports: []ServicePort{
		{Proto: "udp", Port: "1194", Description: "VPN"},
	}},
	{ID: "paperless", Name: "Paperless-ngx", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "8000", Description: "Web interface"},
	}},
	{ID: "pihole", Name: "Pi-hole", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "80", Description: "Web interface"},
		{Proto: "tcp", Port: "53", Description: "DNS"},
		{Proto: "udp", Port: "53", Description: "DNS"},
	}},
	{ID: "plex", Name: "Plex", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "32400", Description: "Media server"},
	}},
	{ID: "portainer", Name: "Portainer", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "9443", Description: "Web interface"},
	}},
	{ID: "postgresql", Name: "PostgreSQL", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "5432", Description: "Database"},
	}},
	{ID: "prometheus", Name: "Prometheus", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "9090", Description: "Web interface"},
	}},
	{ID: "proxmox", Name: "Proxmox VE", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "8006", Description: "Web interface"},
	}},
	{ID: "rdp", Name: "Remote Desktop", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "3389", Description: "RDP"},
	}},
	{ID: "redis", Name: "Redis", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "6379", Description: "Database"},
	}},
	{ID: "samba", Name: "Samba", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "445", Description: "SMB"},
		{Proto: "tcp", Port: "139", Description: "NetBIOS session"},
	}},
	{ID: "ssh", Name: "SSH", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "22", Description: "SSH"},
	}},
	{ID: "syncthing", Name: "Syncthing", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "8384", Description: "Web interface"},
		{Proto: "tcp", Port: "22000", Description: "Sync"},
		{Proto: "udp", Port: "22000", Description: "Sync"},
	}},
	{ID: "unifi", Name: "UniFi Network", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "8443", Description: "Web interface"},
		{Proto: "tcp", Port: "8080", Description: "Device communication"},
	}},
	{ID: "uptimekuma", Name: "Uptime Kuma", Suggest: SuggestPrivate, Ports: []ServicePort{
		{Proto: "tcp", Port: "3001", Description: "Web interface"},
	}},
	{ID: "wireguard", Name: "WireGuard", Suggest: SuggestAnywhere, Ports: []ServicePort{
		{Proto: "udp", Port: "51820", Description: "VPN"},
	}},
}

// ServiceByID finds an entry. The second return is false for an id the catalogue
// no longer knows, which is a rule that keeps working and renders its id as
// plain text — never an error and never a changed rule.
func ServiceByID(id string) (Service, bool) {
	for _, s := range Catalogue {
		if s.ID == id {
			return s, true
		}
	}
	return Service{}, false
}
