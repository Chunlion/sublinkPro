package models

import (
	"fmt"
	"slices"
	"sort"
	"sync/atomic"

	"sublink/cache"
	"sublink/database"
	"sublink/utils"

	"gorm.io/gorm/clause"
)

const defaultGroupAirportSortWeight = 999999

type GroupNodeOrder struct {
	GroupName string `gorm:"primaryKey;size:200"`
	NodeIDs   []int  `gorm:"serializer:json;type:text"`
}

var (
	groupNodeOrderCache       *cache.MapCache[string, GroupNodeOrder]
	groupNodeOrderCacheLoaded atomic.Bool
)

func init() {
	groupNodeOrderCache = cache.NewMapCache(func(g GroupNodeOrder) string { return g.GroupName })
}

// InitGroupNodeOrderCache 初始化分组节点排序缓存
func InitGroupNodeOrderCache() error {
	utils.Info("开始加载分组节点排序到缓存")

	var orders []GroupNodeOrder
	if err := database.DB.Find(&orders).Error; err != nil {
		return err
	}

	groupNodeOrderCache.LoadAll(orders)
	groupNodeOrderCacheLoaded.Store(true)
	utils.Info("分组节点排序缓存初始化完成，共加载 %d 条记录", groupNodeOrderCache.Count())

	cache.Manager.Register("groupNodeOrder", groupNodeOrderCache)
	return nil
}

func resetGroupNodeOrderCacheForTest() {
	groupNodeOrderCache = cache.NewMapCache(func(g GroupNodeOrder) string { return g.GroupName })
	groupNodeOrderCacheLoaded.Store(false)
}

func normalizeGroupNodeIDs(groupName string, nodeIDs []int) ([]int, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	validSet := make(map[int]struct{})
	if groupNodes := nodeCache.GetByIndex("group", groupName); len(groupNodes) > 0 {
		for _, n := range groupNodes {
			validSet[n.ID] = struct{}{}
		}
	} else if nodeCache.Count() == 0 {
		var ids []int
		if err := database.DB.Model(&Node{}).
			Where(clause.Eq{Column: clause.Column{Table: "nodes", Name: "group"}, Value: groupName}).
			Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
		for _, id := range ids {
			validSet[id] = struct{}{}
		}
	}

	seen := make(map[int]struct{}, len(nodeIDs))
	cleaned := make([]int, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		if id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		if _, ok := validSet[id]; !ok {
			continue
		}
		seen[id] = struct{}{}
		cleaned = append(cleaned, id)
	}

	if len(cleaned) == 0 {
		return nil, fmt.Errorf("所选节点不属于该分组或已不存在")
	}
	return cleaned, nil
}

func SaveGroupNodeOrder(groupName string, nodeIDs []int) error {
	if groupName == "" {
		return fmt.Errorf("分组名称不能为空")
	}
	if len(nodeIDs) == 0 {
		return DeleteGroupNodeOrder(groupName)
	}

	cleaned, err := normalizeGroupNodeIDs(groupName, nodeIDs)
	if err != nil {
		return err
	}

	record := GroupNodeOrder{GroupName: groupName, NodeIDs: cleaned}
	if err := database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "group_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"node_ids"}),
	}).Create(&record).Error; err != nil {
		return err
	}

	groupNodeOrderCache.Set(groupName, GroupNodeOrder{
		GroupName: groupName,
		NodeIDs:   slices.Clone(cleaned),
	})
	return nil
}

func DeleteGroupNodeOrder(groupName string) error {
	if groupName == "" {
		return nil
	}
	if err := database.DB.Where("group_name = ?", groupName).Delete(&GroupNodeOrder{}).Error; err != nil {
		return err
	}
	groupNodeOrderCache.Delete(groupName)
	return nil
}

func GetGroupNodeOrders(groupNames []string) (map[string][]int, error) {
	if len(groupNames) == 0 {
		return nil, nil
	}
	if groupNodeOrderCacheLoaded.Load() {
		byGroup := make(map[string][]int, len(groupNames))
		for _, name := range groupNames {
			if order, ok := groupNodeOrderCache.Get(name); ok && len(order.NodeIDs) > 0 {
				byGroup[name] = slices.Clone(order.NodeIDs)
			}
		}
		return byGroup, nil
	}
	var orders []GroupNodeOrder
	if err := database.DB.Where("group_name IN ?", groupNames).Find(&orders).Error; err != nil {
		return nil, err
	}
	byGroup := make(map[string][]int, len(orders))
	for _, order := range orders {
		if len(order.NodeIDs) > 0 {
			byGroup[order.GroupName] = order.NodeIDs
		}
	}
	return byGroup, nil
}

func getGroupNodeOrderGroupSet() (map[string]bool, error) {
	configured := make(map[string]bool)
	if groupNodeOrderCacheLoaded.Load() {
		for _, order := range groupNodeOrderCache.GetAll() {
			if len(order.NodeIDs) > 0 {
				configured[order.GroupName] = true
			}
		}
		return configured, nil
	}
	var orders []GroupNodeOrder
	if err := database.DB.Find(&orders).Error; err != nil {
		return nil, err
	}
	for _, order := range orders {
		if len(order.NodeIDs) > 0 {
			configured[order.GroupName] = true
		}
	}
	return configured, nil
}

// PruneGroupNodeOrders 从指定分组的自定义节点排序中剔除已删除或移出的节点 ID；若清空则删除整条配置。
func PruneGroupNodeOrders(groupToRemovedIDs map[string]map[int]struct{}) {
	if len(groupToRemovedIDs) == 0 {
		return
	}

	groupNames := make([]string, 0, len(groupToRemovedIDs))
	for groupName, removed := range groupToRemovedIDs {
		if groupName != "" && len(removed) > 0 {
			groupNames = append(groupNames, groupName)
		}
	}
	if len(groupNames) == 0 {
		return
	}

	orders, err := GetGroupNodeOrders(groupNames)
	if err != nil {
		utils.Warn("读取分组节点排序以清理失效节点失败: %v", err)
		return
	}

	for _, groupName := range groupNames {
		existing := orders[groupName]
		if len(existing) == 0 {
			continue
		}
		removed := groupToRemovedIDs[groupName]
		filtered := make([]int, 0, len(existing))
		changed := false
		for _, id := range existing {
			if _, drop := removed[id]; drop {
				changed = true
				continue
			}
			filtered = append(filtered, id)
		}
		if !changed {
			continue
		}
		if len(filtered) == 0 {
			if err := DeleteGroupNodeOrder(groupName); err != nil {
				utils.Warn("清理空的分组 %s 节点排序失败: %v", groupName, err)
			}
			continue
		}
		record := GroupNodeOrder{GroupName: groupName, NodeIDs: filtered}
		if err := database.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "group_name"}},
			DoUpdates: clause.AssignmentColumns([]string{"node_ids"}),
		}).Create(&record).Error; err != nil {
			utils.Warn("更新分组 %s 节点排序失败: %v", groupName, err)
			continue
		}
		groupNodeOrderCache.Set(groupName, GroupNodeOrder{
			GroupName: groupName,
			NodeIDs:   slices.Clone(filtered),
		})
	}
}

func SortGroupNodes(groupName string, nodes []Node, nodeIDs []int) []Node {
	SortNodesByAirport(nodes, GetGroupAirportSortMap(groupName))
	if len(nodeIDs) == 0 || len(nodes) <= 1 {
		return nodes
	}
	ranks := make(map[int]int, len(nodeIDs))
	for i, id := range nodeIDs {
		if id <= 0 {
			continue
		}
		if _, exists := ranks[id]; !exists {
			ranks[id] = i + 1
		}
	}
	if len(ranks) == 0 {
		return nodes
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		left, right := ranks[nodes[i].ID], ranks[nodes[j].ID]
		return left > 0 && (right == 0 || left < right)
	})
	return nodes
}

// SortNodesByAirport 对同一分组下的节点做“按机场聚合”的稳定排序，避免不同机场节点穿插。
//
// 规则：
// 1) 机场优先级：按 airportSortMap（airportID -> sortWeight，值越小越靠前）；未配置的机场使用默认权重 defaultGroupAirportSortWeight。
// 2) 同权重下：按首次出现顺序（基于输入切片顺序）保持稳定，尽量不改变既有机场块的相对位置。
// 3) 单机场内部：优先按 SourceSort（上游订阅顺序，从 1 开始）排序；SourceSort=0 视为未初始化，保持原顺序并排在最后。
//
// 说明：
// - 输入 nodes 建议已是稳定顺序（例如按 nodes.id ASC），以便“首次出现顺序”可复现。
// - 该函数不会做去重；去重由上层逻辑（订阅展开时的 nodeMap / 去重规则）负责。
func SortNodesByAirport(nodes []Node, airportSortMap map[int]int) []Node {
	if len(nodes) <= 1 {
		return nodes
	}

	// 每个机场的首次出现序号（用于同权重下保持稳定）
	firstRankBySource := make(map[int]int)
	nextRank := 0
	for _, n := range nodes {
		if _, ok := firstRankBySource[n.SourceID]; ok {
			continue
		}
		firstRankBySource[n.SourceID] = nextRank
		nextRank++
	}

	sort.SliceStable(nodes, func(i, j int) bool {
		si := nodes[i].SourceID
		sj := nodes[j].SourceID

		// 先按机场排序权重
		wi, okI := airportSortMap[si]
		wj, okJ := airportSortMap[sj]
		if !okI {
			wi = defaultGroupAirportSortWeight
		}
		if !okJ {
			wj = defaultGroupAirportSortWeight
		}
		if wi != wj {
			return wi < wj
		}

		// 同权重下按首次出现顺序
		ri := firstRankBySource[si]
		rj := firstRankBySource[sj]
		if ri != rj {
			return ri < rj
		}

		// 同机场内部按 SourceSort（上游顺序）排序；未初始化的（0）排最后
		is := nodes[i].SourceSort
		js := nodes[j].SourceSort
		if is == 0 && js == 0 {
			return false
		}
		if is == 0 {
			return false
		}
		if js == 0 {
			return true
		}
		if is != js {
			return is < js
		}

		return nodes[i].ID < nodes[j].ID
	})

	return nodes
}
