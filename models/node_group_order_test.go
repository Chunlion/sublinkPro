package models

import (
	"slices"
	"testing"

	"sublink/database"
	"sublink/internal/testutil"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupGroupNodeOrderTestDB(t *testing.T) {
	t.Helper()

	oldDB := database.DB
	oldDialect := database.Dialect
	oldInitialized := database.IsInitialized
	oldNodeCache := nodeCache
	oldAirportCache := airportCache
	oldGroupAirportSortCache := groupAirportSortCache

	db, err := gorm.Open(sqlite.Open(testutil.UniqueMemoryDSN(t, "node_group_order_test")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&Node{}, &Airport{}, &GroupAirportSort{}, &GroupNodeOrder{}, &SubcriptionNode{}); err != nil {
		t.Fatalf("auto migrate tables: %v", err)
	}

	database.DB = db
	database.Dialect = database.DialectSQLite
	database.IsInitialized = true
	resetNodeCacheForTest()
	resetAirportCacheForTest()
	resetGroupNodeOrderCacheForTest()
	if err := InitGroupAirportSortCache(); err != nil {
		t.Fatalf("init group airport sort cache: %v", err)
	}
	if err := InitGroupNodeOrderCache(); err != nil {
		t.Fatalf("init group node order cache: %v", err)
	}

	t.Cleanup(func() {
		database.DB = oldDB
		database.Dialect = oldDialect
		database.IsInitialized = oldInitialized
		nodeCache = oldNodeCache
		airportCache = oldAirportCache
		groupAirportSortCache = oldGroupAirportSortCache
		resetGroupNodeOrderCacheForTest()
		testutil.CloseDB(t, db)
	})
}

func addGroupOrderTestNode(t *testing.T, name, group string, sourceID int) Node {
	t.Helper()
	n := Node{
		Name:     name,
		LinkName: name,
		Group:    group,
		SourceID: sourceID,
		Link:     "ss://YWVzLTEyOC1nY206cGFzc0BleGFtcGxlLmNvbTo0NDM=#" + name,
		Protocol: "ss",
	}
	if err := n.Add(); err != nil {
		t.Fatalf("add node %s: %v", name, err)
	}
	return n
}

func TestSaveGroupNodeOrderDeduplicatesAndFiltersInvalidIDs(t *testing.T) {
	setupGroupNodeOrderTestDB(t)

	n1 := addGroupOrderTestNode(t, "g1-a", "g1", 0)
	n2 := addGroupOrderTestNode(t, "g1-b", "g1", 0)
	other := addGroupOrderTestNode(t, "g2-a", "g2", 0)

	if err := SaveGroupNodeOrder("g1", []int{n2.ID, n2.ID, other.ID, -1, n1.ID}); err != nil {
		t.Fatalf("save group node order: %v", err)
	}

	orders, err := GetGroupNodeOrders([]string{"g1"})
	if err != nil {
		t.Fatalf("get group node orders: %v", err)
	}
	if want := []int{n2.ID, n1.ID}; !slices.Equal(orders["g1"], want) {
		t.Fatalf("orders[g1] = %v, want %v", orders["g1"], want)
	}

	if err := SaveGroupNodeOrder("g1", []int{other.ID}); err == nil {
		t.Fatal("expected error when saving nodeIds that do not belong to group")
	}
	orders, err = GetGroupNodeOrders([]string{"g1"})
	if err != nil {
		t.Fatalf("get group node orders after rejected save: %v", err)
	}
	if want := []int{n2.ID, n1.ID}; !slices.Equal(orders["g1"], want) {
		t.Fatalf("orders[g1] changed after invalid save: %v", orders["g1"])
	}
}

func TestGroupNodeOrderPrunesOnNodeLifecycleAndCleanup(t *testing.T) {
	setupGroupNodeOrderTestDB(t)

	n1 := addGroupOrderTestNode(t, "n1", "g1", 10)
	n2 := addGroupOrderTestNode(t, "n2", "g1", 10)
	n3 := addGroupOrderTestNode(t, "n3", "g1", 20)
	n4 := addGroupOrderTestNode(t, "n4", "g1", 20)

	if err := SaveGroupNodeOrder("g1", []int{n4.ID, n3.ID, n2.ID, n1.ID}); err != nil {
		t.Fatalf("save group node order: %v", err)
	}

	// Single node delete prunes n4.ID
	if err := (&n4).Del(); err != nil {
		t.Fatalf("delete n4: %v", err)
	}
	orders, _ := GetGroupNodeOrders([]string{"g1"})
	if want := []int{n3.ID, n2.ID, n1.ID}; !slices.Equal(orders["g1"], want) {
		t.Fatalf("after Del: %v, want %v", orders["g1"], want)
	}

	// Moving n3 to another group prunes n3.ID
	n3.Group = "g2"
	if err := (&n3).Update(); err != nil {
		t.Fatalf("move n3: %v", err)
	}
	orders, _ = GetGroupNodeOrders([]string{"g1"})
	if want := []int{n2.ID, n1.ID}; !slices.Equal(orders["g1"], want) {
		t.Fatalf("after Update group: %v, want %v", orders["g1"], want)
	}

	// BatchUpdateGroup moves n2 out of g1
	if err := BatchUpdateGroup([]int{n2.ID}, "g2"); err != nil {
		t.Fatalf("batch update group: %v", err)
	}
	orders, _ = GetGroupNodeOrders([]string{"g1"})
	if want := []int{n1.ID}; !slices.Equal(orders["g1"], want) {
		t.Fatalf("after BatchUpdateGroup: %v, want %v", orders["g1"], want)
	}

	// BatchDel deletes last remaining node n1 and deletes empty GroupNodeOrder record
	if err := BatchDel([]int{n1.ID}); err != nil {
		t.Fatalf("batch del n1: %v", err)
	}
	var count int64
	if err := database.DB.Model(&GroupNodeOrder{}).Where("group_name = ?", "g1").Count(&count).Error; err != nil {
		t.Fatalf("count g1 orders: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected empty GroupNodeOrder row to be deleted, count = %d", count)
	}

	// CleanupGroupSortRecords removes both GroupAirportSort and GroupNodeOrder
	n5 := addGroupOrderTestNode(t, "n5", "g3", 30)
	if err := SaveGroupAirportSorts("g3", []AirportSortItem{{AirportID: 30, Sort: 0}}); err != nil {
		t.Fatalf("save airport sort: %v", err)
	}
	if err := SaveGroupNodeOrder("g3", []int{n5.ID}); err != nil {
		t.Fatalf("save g3 node order: %v", err)
	}
	CleanupGroupSortRecords("g3")
	orders, _ = GetGroupNodeOrders([]string{"g3"})
	if len(orders["g3"]) != 0 {
		t.Fatalf("expected g3 node order cleared, got %v", orders["g3"])
	}
	if len(GetGroupAirportSortMap("g3")) != 0 {
		t.Fatal("expected g3 airport sort cleared")
	}
}

func TestSortNodesByAirport_DefaultOrder(t *testing.T) {
	nodes := []Node{
		{ID: 1, SourceID: 2, Name: "A-3", SourceSort: 3},
		{ID: 2, SourceID: 2, Name: "A-1", SourceSort: 1},
		{ID: 3, SourceID: 4, Name: "B-1", SourceSort: 1},
		{ID: 4, SourceID: 3, Name: "C-1", SourceSort: 2},
		{ID: 5, SourceID: 3, Name: "C-0", SourceSort: 0},
		{ID: 6, SourceID: 4, Name: "B-2", SourceSort: 2},
		{ID: 7, SourceID: 2, Name: "A-2", SourceSort: 2},
	}

	got := SortNodesByAirport(nodes, nil)

	wantNames := []string{"A-1", "A-2", "A-3", "B-1", "B-2", "C-1", "C-0"}
	if len(got) != len(wantNames) {
		t.Fatalf("len(got)=%d, want=%d", len(got), len(wantNames))
	}
	for i, want := range wantNames {
		if got[i].Name != want {
			t.Fatalf("name mismatch at %d: got=%s want=%s", i, got[i].Name, want)
		}
	}
}

// 所有节点 SourceSort=0（纯历史数据），验证退化到按机场聚合 + ID 稳定排序
func TestSortNodesByAirport_AllZeroSourceSort(t *testing.T) {
	nodes := []Node{
		{ID: 1, SourceID: 2, Name: "A-1", SourceSort: 0},
		{ID: 2, SourceID: 4, Name: "B-1", SourceSort: 0},
		{ID: 3, SourceID: 2, Name: "A-2", SourceSort: 0},
		{ID: 4, SourceID: 3, Name: "C-1", SourceSort: 0},
		{ID: 5, SourceID: 4, Name: "B-2", SourceSort: 0},
		{ID: 6, SourceID: 3, Name: "C-2", SourceSort: 0},
	}

	got := SortNodesByAirport(nodes, nil)

	// 机场首次出现顺序：SourceID=2(A) → 4(B) → 3(C)
	// 同机场内 SourceSort 全为 0，SliceStable 保持输入顺序（即 ID 升序）
	wantNames := []string{"A-1", "A-2", "B-1", "B-2", "C-1", "C-2"}
	if len(got) != len(wantNames) {
		t.Fatalf("len(got)=%d, want=%d", len(got), len(wantNames))
	}
	for i, want := range wantNames {
		if got[i].Name != want {
			t.Fatalf("name mismatch at %d: got=%s want=%s", i, got[i].Name, want)
		}
	}
}

func TestSortNodesByAirport_CustomAirportSort(t *testing.T) {
	nodes := []Node{
		{ID: 1, SourceID: 2, Name: "A-3", SourceSort: 3},
		{ID: 2, SourceID: 2, Name: "A-1", SourceSort: 1},
		{ID: 3, SourceID: 4, Name: "B-1", SourceSort: 1},
		{ID: 4, SourceID: 3, Name: "C-1", SourceSort: 2},
		{ID: 5, SourceID: 3, Name: "C-0", SourceSort: 0},
		{ID: 6, SourceID: 4, Name: "B-2", SourceSort: 2},
		{ID: 7, SourceID: 2, Name: "A-2", SourceSort: 2},
	}

	airportSortMap := map[int]int{
		3: 0, // C 优先
		2: 1, // A 次之
		// B 未配置 -> 默认排最后
	}
	got := SortNodesByAirport(nodes, airportSortMap)

	wantNames := []string{"C-1", "C-0", "A-1", "A-2", "A-3", "B-1", "B-2"}
	if len(got) != len(wantNames) {
		t.Fatalf("len(got)=%d, want=%d", len(got), len(wantNames))
	}
	for i, want := range wantNames {
		if got[i].Name != want {
			t.Fatalf("name mismatch at %d: got=%s want=%s", i, got[i].Name, want)
		}
	}
}
