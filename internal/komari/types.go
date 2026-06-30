package komari

import (
	"sort"
	"time"
)

type Node struct {
	UUID             string   `json:"uuid"`
	Token            string   `json:"token"`
	Name             string   `json:"name"`
	CPUName          string   `json:"cpu_name"`
	Virtualization   string   `json:"virtualization"`
	Arch             string   `json:"arch"`
	CPUCores         int      `json:"cpu_cores"`
	CPUPhysicalCores int      `json:"cpu_physical_cores"`
	OS               string   `json:"os"`
	KernelVersion    string   `json:"kernel_version"`
	GPUName          string   `json:"gpu_name"`
	IPv4             string   `json:"ipv4"`
	IPv6             string   `json:"ipv6"`
	Region           string   `json:"region"`
	Remark           string   `json:"remark"`
	PublicRemark     string   `json:"public_remark"`
	MemTotal         int64    `json:"mem_total"`
	SwapTotal        int64    `json:"swap_total"`
	DiskTotal        int64    `json:"disk_total"`
	Version          string   `json:"version"`
	Weight           int      `json:"weight"`
	Price            float64  `json:"price"`
	BillingCycle     int      `json:"billing_cycle"`
	AutoRenewal      bool     `json:"auto_renewal"`
	Currency         string   `json:"currency"`
	ExpiredAt        NullTime `json:"expired_at"`
	Group            string   `json:"group"`
	Tags             string   `json:"tags"`
	Hidden           bool     `json:"hidden"`
	TrafficLimit     int64    `json:"traffic_limit"`
	TrafficLimitType string   `json:"traffic_limit_type"`
	CreatedAt        NullTime `json:"created_at"`
	UpdatedAt        NullTime `json:"updated_at"`
}

type Status struct {
	Client          string          `json:"client"`
	Time            NullTime        `json:"time"`
	CPU             float64         `json:"cpu"`
	GPU             float64         `json:"gpu"`
	RAM             int64           `json:"ram"`
	RAMTotal        int64           `json:"ram_total"`
	Swap            int64           `json:"swap"`
	SwapTotal       int64           `json:"swap_total"`
	Load            float64         `json:"load"`
	Load5           float64         `json:"load5"`
	Load15          float64         `json:"load15"`
	Temp            float64         `json:"temp"`
	Disk            int64           `json:"disk"`
	DiskTotal       int64           `json:"disk_total"`
	NetIn           int64           `json:"net_in"`
	NetOut          int64           `json:"net_out"`
	NetTotalUp      int64           `json:"net_total_up"`
	NetTotalDown    int64           `json:"net_total_down"`
	Process         int             `json:"process"`
	Connections     int             `json:"connections"`
	ConnectionsUDP  int             `json:"connections_udp"`
	Online          bool            `json:"online"`
	Uptime          int64           `json:"uptime"`
	Message         string          `json:"message"`
	Ping            map[string]Ping `json:"ping"`
	NetTotalUpAlias int64           `json:"net_total_out"`
	NetDownAlias    int64           `json:"net_total_in"`
}

type Ping struct {
	Name   string  `json:"name"`
	Latest float64 `json:"latest"`
	Avg    float64 `json:"avg"`
	Tail   float64 `json:"tail"`
	Loss   float64 `json:"loss"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

type Snapshot struct {
	Nodes         []Node
	Status        map[string]Status
	FetchedAt     time.Time
	SourceURL     string
	Public        PublicInfo
	Version       VersionInfo
	RPCVersion    string
	Me            MeInfo
	Methods       []string
	LastErr       error
	NodeDetailErr error
	Online        int
	Total         int
	TotalUp       int64
	TotalDown     int64
	SpeedUp       int64
	SpeedDown     int64
	RegionList    []string
}

func NewSnapshot(sourceURL string, public PublicInfo, nodes map[string]Node, status map[string]Status) Snapshot {
	list := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		if node.Hidden {
			continue
		}
		list = append(list, node)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Weight == list[j].Weight {
			return list[i].Name < list[j].Name
		}
		return list[i].Weight < list[j].Weight
	})

	snapshot := Snapshot{
		Nodes:     list,
		Status:    status,
		FetchedAt: time.Now(),
		SourceURL: sourceURL,
		Public:    public,
		Total:     len(list),
	}

	regions := map[string]struct{}{}
	for _, node := range list {
		st, ok := status[node.UUID]
		if !ok {
			continue
		}
		if st.Online {
			snapshot.Online++
			if node.Region != "" {
				regions[node.Region] = struct{}{}
			}
		}
		snapshot.TotalUp += st.NetTotalUp
		snapshot.TotalDown += st.NetTotalDown
		snapshot.SpeedUp += st.NetOut
		snapshot.SpeedDown += st.NetIn
	}
	for region := range regions {
		snapshot.RegionList = append(snapshot.RegionList, region)
	}
	sort.Strings(snapshot.RegionList)

	return snapshot
}

type PublicInfo struct {
	CORSOriginCheckEnabled bool           `json:"cors_origin_check_enabled"`
	Description            string         `json:"description"`
	DisablePasswordLogin   bool           `json:"disable_password_login"`
	OAuthEnable            bool           `json:"oauth_enable"`
	OAuthProvider          string         `json:"oauth_provider"`
	PingRecordPreserveTime int            `json:"ping_record_preserve_time"`
	PrivateSite            bool           `json:"private_site"`
	RecordEnabled          bool           `json:"record_enabled"`
	RecordPreserveTime     int            `json:"record_preserve_time"`
	SiteName               string         `json:"sitename"`
	Theme                  string         `json:"theme"`
	ThemeSettings          map[string]any `json:"theme_settings"`
}

type VersionInfo struct {
	Version string `json:"version"`
	Hash    string `json:"hash"`
}

type MeInfo struct {
	TwoFAEnabled bool   `json:"2fa_enabled"`
	LoggedIn     bool   `json:"logged_in"`
	SSOID        string `json:"sso_id"`
	SSOType      string `json:"sso_type"`
	Username     string `json:"username"`
	UUID         string `json:"uuid"`
}

type RecentStatusResp struct {
	Count   int      `json:"count"`
	Records []Status `json:"records"`
}

type LoadRecordsResp struct {
	Count      int            `json:"count"`
	Records    []Status       `json:"records"`
	From       NullTime       `json:"from"`
	To         NullTime       `json:"to"`
	LoadType   string         `json:"load_type"`
	HasGPUData bool           `json:"has_gpu_data"`
	GPUDevices map[string]any `json:"gpu_devices"`
}

type PingRecordsResp struct {
	Count     int          `json:"count"`
	Records   []PingRecord `json:"records"`
	BasicInfo []BasicInfo  `json:"basic_info"`
	Tasks     []PingTask   `json:"tasks"`
	From      NullTime     `json:"from"`
	To        NullTime     `json:"to"`
}

type PingRecord struct {
	TaskID int      `json:"task_id"`
	Time   NullTime `json:"time"`
	Value  float64  `json:"value"`
	Client string   `json:"client"`
}

type BasicInfo struct {
	Client string  `json:"client"`
	Loss   float64 `json:"loss"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

type PingTask struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Interval  int      `json:"interval"`
	DefaultOn bool     `json:"default_on"`
	Loss      float64  `json:"loss"`
	Min       float64  `json:"min"`
	Max       float64  `json:"max"`
	Avg       float64  `json:"avg"`
	Total     int      `json:"total"`
	Clients   []string `json:"clients"`
}
