package models

import (
	"fmt"
	"sort"
	"sublink/cache"
	"sublink/database"
	"sublink/utils"

	"gorm.io/gorm"
)

// GroupAirportSort 分组内机场排序配置
type GroupAirportSort struct {
	ID        int    `gorm:"primaryKey;autoIncrement" json:"id"`
	GroupName string `gorm:"uniqueIndex:idx_group_airport;size:200" json:"groupName"`
	AirportID int    `gorm:"uniqueIndex:idx_group_airport" json:"airportId"`
	Sort      int    `gorm:"default:0" json:"sort"` // 值越小越靠前
}

// groupAirportSortCache 分组机场排序缓存
var groupAirportSortCache *cache.MapCache[int, GroupAirportSort]

func init() {
	groupAirportSortCache = cache.NewMapCache(func(g GroupAirportSort) int { return g.ID })
	groupAirportSortCache.AddIndex("groupName", func(g GroupAirportSort) string { return g.GroupName })
}

// InitGroupAirportSortCache 初始化分组机场排序缓存
func InitGroupAirportSortCache() error {
	utils.Info("开始加载分组机场排序到缓存")

	var sorts []GroupAirportSort
	if err := database.DB.Find(&sorts).Error; err != nil {
		return err
	}

	groupAirportSortCache.LoadAll(sorts)
	utils.Info("分组机场排序缓存初始化完成，共加载 %d 条记录", groupAirportSortCache.Count())

	cache.Manager.Register("groupAirportSort", groupAirportSortCache)
	return nil
}

// GroupAirportDetail 分组详情中的机场信息
type GroupAirportDetail struct {
	AirportID   int    `json:"airportId"`
	AirportName string `json:"airportName"`
	NodeCount   int    `json:"nodeCount"`
	Sort        int    `json:"sort"`
}

// GroupDetailResponse 分组详情响应
type GroupDetailResponse struct {
	GroupName      string               `json:"groupName"`
	HasAirportSort bool                 `json:"hasAirportSort"`
	HasNodeOrder   bool                 `json:"hasNodeOrder"`
	Airports       []GroupAirportDetail `json:"airports"`
	Nodes          []GroupNodeDetail    `json:"nodes"`
}

type GroupNodeDetail struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	AirportID    int     `json:"airportId"`
	AirportName  string  `json:"airportName"`
	Protocol     string  `json:"protocol"`
	DelayTime    int     `json:"delayTime"`
	Speed        float64 `json:"speed"`
	InSavedOrder bool    `json:"inSavedOrder"`
}

func resolveAirportName(airportID int) string {
	if airportID <= 0 {
		return "手动添加"
	}
	if airport, ok := airportCache.Get(airportID); ok {
		return airport.Name
	}
	return fmt.Sprintf("机场#%d(已删除)", airportID)
}

// GetGroupDetail 获取分组详情（聚合分组内的机场列表及排序）
func GetGroupDetail(groupName string) (*GroupDetailResponse, error) {
	// 从节点缓存按 group 索引获取该分组下所有节点
	groupNodes := nodeCache.GetByIndex("group", groupName)
	if len(groupNodes) == 0 {
		return &GroupDetailResponse{
			GroupName: groupName,
			Airports:  []GroupAirportDetail{},
			Nodes:     []GroupNodeDetail{},
		}, nil
	}

	// 按节点 ID 升序排列，保证"首次出现顺序"与订阅输出一致
	sort.Slice(groupNodes, func(i, j int) bool {
		return groupNodes[i].ID < groupNodes[j].ID
	})

	// 聚合各 SourceID 的节点数量，同时记录首次出现顺序
	airportNodeCount := make(map[int]int)  // airportID -> nodeCount
	firstRankBySource := make(map[int]int) // airportID -> 首次出现序号
	nextRank := 0
	for _, node := range groupNodes {
		airportNodeCount[node.SourceID]++
		if _, ok := firstRankBySource[node.SourceID]; !ok {
			firstRankBySource[node.SourceID] = nextRank
			nextRank++
		}
	}

	// 获取已保存的排序配置
	existingSorts := groupAirportSortCache.GetByIndex("groupName", groupName)
	sortMap := make(map[int]int) // airportID -> sort
	for _, s := range existingSorts {
		sortMap[s.AirportID] = s.Sort
	}

	// 构建机场详情列表
	airports := make([]GroupAirportDetail, 0, len(airportNodeCount))
	for airportID, count := range airportNodeCount {
		airportName := resolveAirportName(airportID)

		sortVal := 999999 // 默认排到最后
		if s, ok := sortMap[airportID]; ok {
			sortVal = s
		}

		airports = append(airports, GroupAirportDetail{
			AirportID:   airportID,
			AirportName: airportName,
			NodeCount:   count,
			Sort:        sortVal,
		})
	}

	// 按 sort 值升序排列；同 sort 值时按首次出现顺序兜底（与订阅输出一致）
	sort.Slice(airports, func(i, j int) bool {
		if airports[i].Sort != airports[j].Sort {
			return airports[i].Sort < airports[j].Sort
		}
		return firstRankBySource[airports[i].AirportID] < firstRankBySource[airports[j].AirportID]
	})

	// 重新编号 sort 值（从 0 开始连续）
	for i := range airports {
		airports[i].Sort = i
	}

	nodeOrders, err := GetGroupNodeOrders([]string{groupName})
	if err != nil {
		return nil, err
	}
	savedNodeIDs := nodeOrders[groupName]
	savedSet := make(map[int]bool, len(savedNodeIDs))
	for _, id := range savedNodeIDs {
		if id > 0 {
			savedSet[id] = true
		}
	}

	groupNodes = SortGroupNodes(groupName, groupNodes, savedNodeIDs)
	nodes := make([]GroupNodeDetail, 0, len(groupNodes))
	for _, node := range groupNodes {
		proto := node.Protocol
		if proto == "" && node.Link != "" {
			proto = utils.GetProtocolFromLink(node.Link)
		}
		nodes = append(nodes, GroupNodeDetail{
			ID:           node.ID,
			Name:         node.EffectiveName(),
			AirportID:    node.SourceID,
			AirportName:  resolveAirportName(node.SourceID),
			Protocol:     proto,
			DelayTime:    node.DelayTime,
			Speed:        node.Speed,
			InSavedOrder: savedSet[node.ID],
		})
	}

	return &GroupDetailResponse{
		GroupName:      groupName,
		HasAirportSort: len(existingSorts) > 0,
		HasNodeOrder:   len(savedNodeIDs) > 0,
		Airports:       airports,
		Nodes:          nodes,
	}, nil
}

// AirportSortItem 保存请求中的单条排序项
type AirportSortItem struct {
	AirportID int `json:"airportId"`
	Sort      int `json:"sort"`
}

type SaveGroupSortRequest struct {
	GroupName    string            `json:"groupName"`
	AirportSorts []AirportSortItem `json:"airportSorts"`
	NodeIDs      []int             `json:"nodeIds"`
}

// SaveGroupAirportSorts 保存分组内机场排序（事务删旧 + 批量插新 + 更新缓存）
func SaveGroupAirportSorts(groupName string, sorts []AirportSortItem) error {
	if groupName == "" {
		return fmt.Errorf("分组名称不能为空")
	}

	err := database.WithTransaction(func(tx *gorm.DB) error {
		// 删除该分组的旧排序记录
		if err := tx.Where("group_name = ?", groupName).Delete(&GroupAirportSort{}).Error; err != nil {
			return err
		}

		// 批量插入新记录
		if len(sorts) > 0 {
			records := make([]GroupAirportSort, 0, len(sorts))
			for _, s := range sorts {
				records = append(records, GroupAirportSort{
					GroupName: groupName,
					AirportID: s.AirportID,
					Sort:      s.Sort,
				})
			}
			if err := tx.Create(&records).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	// 更新缓存：先删除旧的，再加载新的
	oldEntries := groupAirportSortCache.GetByIndex("groupName", groupName)
	for _, entry := range oldEntries {
		groupAirportSortCache.Delete(entry.ID)
	}

	// 从数据库重新加载该分组的记录到缓存
	var newRecords []GroupAirportSort
	if err := database.DB.Where("group_name = ?", groupName).Find(&newRecords).Error; err != nil {
		utils.Warn("重新加载分组 %s 的排序缓存失败: %v", groupName, err)
	} else {
		for _, r := range newRecords {
			groupAirportSortCache.Set(r.ID, r)
		}
	}

	return nil
}

// GetGroupAirportSortMap 获取分组内机场排序映射，供 GetSub 调用
// 返回 airportID -> sortWeight
func GetGroupAirportSortMap(groupName string) map[int]int {
	entries := groupAirportSortCache.GetByIndex("groupName", groupName)
	if len(entries) == 0 {
		return nil
	}

	result := make(map[int]int, len(entries))
	for _, e := range entries {
		result[e.AirportID] = e.Sort
	}
	return result
}

// GetAllGroupNames 获取所有分组名称（从节点缓存获取）
func GetAllGroupNames() []string {
	groups := nodeCache.GetDistinctIndexValues("group")
	// 过滤空分组
	result := make([]string, 0, len(groups))
	for _, g := range groups {
		if g != "" {
			result = append(result, g)
		}
	}
	sort.Strings(result)
	return result
}

// GroupInfo 分组概要信息
type GroupInfo struct {
	GroupName      string `json:"groupName"`
	NodeCount      int    `json:"nodeCount"`
	AirportCount   int    `json:"airportCount"`
	HasSortConfig  bool   `json:"hasSortConfig"`
	HasAirportSort bool   `json:"hasAirportSort"`
	HasNodeOrder   bool   `json:"hasNodeOrder"`
}

// GetAllGroupInfos 获取所有分组的概要信息
func GetAllGroupInfos() ([]GroupInfo, error) {
	groupNames := GetAllGroupNames()
	airportSortGroups := make(map[string]bool)
	all := groupAirportSortCache.GetAll()
	for _, s := range all {
		airportSortGroups[s.GroupName] = true
	}
	nodeOrderGroups, err := getGroupNodeOrderGroupSet()
	if err != nil {
		return nil, err
	}

	infos := make([]GroupInfo, 0, len(groupNames))
	for _, name := range groupNames {
		groupNodes := nodeCache.GetByIndex("group", name)
		airportSet := make(map[int]bool)
		for _, node := range groupNodes {
			airportSet[node.SourceID] = true
		}

		hasAirportSort := airportSortGroups[name]
		hasNodeOrder := nodeOrderGroups[name]
		infos = append(infos, GroupInfo{
			GroupName:      name,
			NodeCount:      len(groupNodes),
			AirportCount:   len(airportSet),
			HasSortConfig:  hasAirportSort || hasNodeOrder,
			HasAirportSort: hasAirportSort,
			HasNodeOrder:   hasNodeOrder,
		})
	}
	return infos, nil
}

// CleanupAirportSortRecords 清理指定机场的所有排序记录（机场删除时调用）
func CleanupAirportSortRecords(airportID int) {
	// 从缓存中找出该机场的所有排序记录并删除
	all := groupAirportSortCache.GetAll()
	for _, entry := range all {
		if entry.AirportID == airportID {
			groupAirportSortCache.Delete(entry.ID)
		}
	}
	// 从数据库删除
	if err := database.DB.Where("airport_id = ?", airportID).Delete(&GroupAirportSort{}).Error; err != nil {
		utils.Warn("清理机场 %d 的排序记录失败: %v", airportID, err)
	}
}

// CleanupGroupSortRecords 清理指定分组的所有排序记录（分组被清空时可调用）
func CleanupGroupSortRecords(groupName string) {
	entries := groupAirportSortCache.GetByIndex("groupName", groupName)
	for _, entry := range entries {
		groupAirportSortCache.Delete(entry.ID)
	}
	if err := database.DB.Where("group_name = ?", groupName).Delete(&GroupAirportSort{}).Error; err != nil {
		utils.Warn("清理分组 %s 的排序记录失败: %v", groupName, err)
	}
	if err := DeleteGroupNodeOrder(groupName); err != nil {
		utils.Warn("清理分组 %s 的节点排序记录失败: %v", groupName, err)
	}
}
