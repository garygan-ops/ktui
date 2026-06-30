package komari

import "encoding/json"

type NodeMap map[string]Node

func (m *NodeMap) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*m = nil
		return nil
	}

	var mapped map[string]Node
	if err := json.Unmarshal(data, &mapped); err == nil {
		*m = NodeMap(mapped)
		return nil
	}

	var list []Node
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	out := make(map[string]Node, len(list))
	for _, node := range list {
		if node.UUID == "" {
			continue
		}
		out[node.UUID] = node
	}
	*m = NodeMap(out)
	return nil
}

func mergeNodeDetails(nodes map[string]Node, detailed map[string]Node) {
	for uuid, detail := range detailed {
		node, ok := nodes[uuid]
		if !ok {
			nodes[uuid] = detail
			continue
		}
		nodes[uuid] = mergeNode(node, detail)
	}
}

func mergeNode(node Node, detail Node) Node {
	if detail.Token != "" {
		node.Token = detail.Token
	}
	if detail.Name != "" {
		node.Name = detail.Name
	}
	if detail.CPUName != "" {
		node.CPUName = detail.CPUName
	}
	if detail.Virtualization != "" {
		node.Virtualization = detail.Virtualization
	}
	if detail.Arch != "" {
		node.Arch = detail.Arch
	}
	if detail.CPUCores != 0 {
		node.CPUCores = detail.CPUCores
	}
	if detail.CPUPhysicalCores != 0 {
		node.CPUPhysicalCores = detail.CPUPhysicalCores
	}
	if detail.OS != "" {
		node.OS = detail.OS
	}
	if detail.KernelVersion != "" {
		node.KernelVersion = detail.KernelVersion
	}
	if detail.GPUName != "" {
		node.GPUName = detail.GPUName
	}
	if detail.IPv4 != "" {
		node.IPv4 = detail.IPv4
	}
	if detail.IPv6 != "" {
		node.IPv6 = detail.IPv6
	}
	if detail.Region != "" {
		node.Region = detail.Region
	}
	if detail.Remark != "" {
		node.Remark = detail.Remark
	}
	if detail.PublicRemark != "" {
		node.PublicRemark = detail.PublicRemark
	}
	if detail.MemTotal != 0 {
		node.MemTotal = detail.MemTotal
	}
	if detail.SwapTotal != 0 {
		node.SwapTotal = detail.SwapTotal
	}
	if detail.DiskTotal != 0 {
		node.DiskTotal = detail.DiskTotal
	}
	if detail.Version != "" {
		node.Version = detail.Version
	}
	if detail.Weight != 0 {
		node.Weight = detail.Weight
	}
	if detail.Price != 0 {
		node.Price = detail.Price
	}
	if detail.BillingCycle != 0 {
		node.BillingCycle = detail.BillingCycle
	}
	if detail.AutoRenewal {
		node.AutoRenewal = true
	}
	if detail.Currency != "" {
		node.Currency = detail.Currency
	}
	if detail.ExpiredAt.Valid {
		node.ExpiredAt = detail.ExpiredAt
	}
	if detail.Group != "" {
		node.Group = detail.Group
	}
	if detail.Tags != "" {
		node.Tags = detail.Tags
	}
	if detail.Hidden {
		node.Hidden = true
	}
	if detail.TrafficLimit != 0 {
		node.TrafficLimit = detail.TrafficLimit
	}
	if detail.TrafficLimitType != "" {
		node.TrafficLimitType = detail.TrafficLimitType
	}
	if detail.CreatedAt.Valid {
		node.CreatedAt = detail.CreatedAt
	}
	if detail.UpdatedAt.Valid {
		node.UpdatedAt = detail.UpdatedAt
	}
	return node
}

func (r *LoadRecordsResp) UnmarshalJSON(data []byte) error {
	var raw struct {
		Count      int             `json:"count"`
		Records    json.RawMessage `json:"records"`
		From       NullTime        `json:"from"`
		To         NullTime        `json:"to"`
		LoadType   string          `json:"load_type"`
		HasGPUData bool            `json:"has_gpu_data"`
		GPUDevices map[string]any  `json:"gpu_devices"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*r = LoadRecordsResp{
		Count:      raw.Count,
		From:       raw.From,
		To:         raw.To,
		LoadType:   raw.LoadType,
		HasGPUData: raw.HasGPUData,
		GPUDevices: raw.GPUDevices,
	}

	if len(raw.Records) == 0 || string(raw.Records) == "null" {
		return nil
	}
	var list []Status
	if err := json.Unmarshal(raw.Records, &list); err == nil {
		r.Records = list
		return nil
	}

	var mapped map[string][]Status
	if err := json.Unmarshal(raw.Records, &mapped); err != nil {
		return err
	}
	for _, records := range mapped {
		r.Records = append(r.Records, records...)
	}
	return nil
}
