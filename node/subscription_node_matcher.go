package node

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"sublink/models"
	"sublink/node/protocol"
)

type subscriptionNodeMatcher struct {
	byLink                      map[string][]models.Node
	byHash                      map[string][]models.Node
	byHashName                  map[string]map[string][]models.Node
	byHashSort                  map[string]map[int][]models.Node
	byStableIdentity            map[string][]models.Node
	bySourceSort                map[int][]models.Node
	stableIdentityCounts        map[string]int
	currentStableIdentityCounts map[string]int
	currentHashCounts           map[string]int
	currentNodeCount            int
	nodes                       []models.Node
	matchedExistingIDs          map[int]bool
}

type subscriptionNodeMatchCandidate struct {
	node  models.Node
	score int
}

const (
	subscriptionNodeExactLinkScore      = 5000
	subscriptionNodeContentHashScore    = 3000
	subscriptionNodeStableIdentityScore = 2200
	subscriptionNodeSourceSortScore     = 700
	subscriptionNodeSingleNodeScore     = 300
)

func newSubscriptionNodeMatcher(nodes []models.Node, currentHashCounts map[string]int, currentStableIdentityCounts ...map[string]int) *subscriptionNodeMatcher {
	currentIdentityCounts := map[string]int{}
	if len(currentStableIdentityCounts) > 0 && currentStableIdentityCounts[0] != nil {
		currentIdentityCounts = currentStableIdentityCounts[0]
	}
	matcher := &subscriptionNodeMatcher{
		byLink:                      make(map[string][]models.Node),
		byHash:                      make(map[string][]models.Node),
		byHashName:                  make(map[string]map[string][]models.Node),
		byHashSort:                  make(map[string]map[int][]models.Node),
		byStableIdentity:            make(map[string][]models.Node),
		bySourceSort:                make(map[int][]models.Node),
		stableIdentityCounts:        make(map[string]int),
		currentStableIdentityCounts: currentIdentityCounts,
		currentHashCounts:           currentHashCounts,
		nodes:                       append([]models.Node(nil), nodes...),
		matchedExistingIDs:          make(map[int]bool, len(nodes)),
	}
	for _, count := range currentHashCounts {
		matcher.currentNodeCount += count
	}
	for _, node := range nodes {
		if link := strings.TrimSpace(node.Link); link != "" {
			matcher.byLink[link] = append(matcher.byLink[link], node)
			if identityKey := subscriptionNodeStableIdentityKey(link); identityKey != "" {
				matcher.byStableIdentity[identityKey] = append(matcher.byStableIdentity[identityKey], node)
				matcher.stableIdentityCounts[identityKey]++
			}
		}
		if node.SourceSort > 0 {
			matcher.bySourceSort[node.SourceSort] = append(matcher.bySourceSort[node.SourceSort], node)
		}
		if node.ContentHash == "" {
			continue
		}
		matcher.byHash[node.ContentHash] = append(matcher.byHash[node.ContentHash], node)

		name := subscriptionNodeOriginalName(node)
		if name != "" {
			if matcher.byHashName[node.ContentHash] == nil {
				matcher.byHashName[node.ContentHash] = make(map[string][]models.Node)
			}
			matcher.byHashName[node.ContentHash][name] = append(matcher.byHashName[node.ContentHash][name], node)
		}
		if node.SourceSort > 0 {
			if matcher.byHashSort[node.ContentHash] == nil {
				matcher.byHashSort[node.ContentHash] = make(map[int][]models.Node)
			}
			matcher.byHashSort[node.ContentHash][node.SourceSort] = append(matcher.byHashSort[node.ContentHash][node.SourceSort], node)
		}
	}
	return matcher
}

func subscriptionNodeOriginalName(node models.Node) string {
	name := strings.TrimSpace(node.LinkName)
	if name == "" {
		name = strings.TrimSpace(node.Name)
	}
	return name
}

func (matcher *subscriptionNodeMatcher) isMatched(id int) bool {
	if matcher == nil {
		return false
	}
	return matcher.matchedExistingIDs[id]
}

func (matcher *subscriptionNodeMatcher) match(link, contentHash, linkName string, sourceSort int) (models.Node, bool) {
	if matcher == nil {
		return models.Node{}, false
	}

	link = strings.TrimSpace(link)
	linkName = strings.TrimSpace(linkName)
	stableIdentityKey := subscriptionNodeStableIdentityKey(link)

	candidates := make(map[int]subscriptionNodeMatchCandidate)
	matcher.addCandidates(candidates, matcher.byLink[link], link, contentHash, linkName, sourceSort, 0)
	if contentHash != "" {
		matcher.addCandidates(candidates, matcher.byHashName[contentHash][linkName], link, contentHash, linkName, sourceSort, 0)
		if sourceSort > 0 {
			matcher.addCandidates(candidates, matcher.byHashSort[contentHash][sourceSort], link, contentHash, linkName, sourceSort, 0)
		}
		matcher.addCandidates(candidates, matcher.byHash[contentHash], link, contentHash, linkName, sourceSort, 0)
	}
	if stableIdentityKey != "" {
		if matcher.stableIdentityCounts[stableIdentityKey] == 1 && matcher.currentStableIdentityCounts[stableIdentityKey] == 1 {
			matcher.addCandidates(candidates, matcher.byStableIdentity[stableIdentityKey], link, contentHash, linkName, sourceSort, subscriptionNodeStableIdentityScore)
		} else {
			matcher.addStableIdentitySourceSortCandidate(candidates, stableIdentityKey, link, contentHash, linkName, sourceSort)
		}
	}
	if sourceSort > 0 {
		matcher.addUniqueSourceSortCandidate(candidates, link, contentHash, linkName, sourceSort)
	}
	matcher.addSingleNodeFallbackCandidate(candidates, link, contentHash, linkName, sourceSort)

	best, ok := bestSubscriptionNodeMatch(candidates)
	if !ok {
		return models.Node{}, false
	}
	matcher.matchedExistingIDs[best.ID] = true
	return best, true
}

func (matcher *subscriptionNodeMatcher) addCandidates(candidates map[int]subscriptionNodeMatchCandidate, nodes []models.Node, link, contentHash, linkName string, sourceSort int, baseScore int) {
	hashShrinking := matcher.hashShrinking(contentHash)
	for _, node := range nodes {
		if matcher.matchedExistingIDs[node.ID] {
			continue
		}
		score := baseScore + scoreSubscriptionNodeMatch(node, link, contentHash, linkName, sourceSort, hashShrinking)
		if score <= 0 {
			continue
		}
		if existing, ok := candidates[node.ID]; !ok || score > existing.score {
			candidates[node.ID] = subscriptionNodeMatchCandidate{node: node, score: score}
		}
	}
}

func (matcher *subscriptionNodeMatcher) addUniqueSourceSortCandidate(candidates map[int]subscriptionNodeMatchCandidate, link, contentHash, linkName string, sourceSort int) {
	unmatched := matcher.unmatchedNodes(matcher.bySourceSort[sourceSort])
	if len(unmatched) != 1 {
		return
	}
	matcher.addCandidates(candidates, unmatched, link, contentHash, linkName, sourceSort, subscriptionNodeSourceSortScore)
}

func (matcher *subscriptionNodeMatcher) addStableIdentitySourceSortCandidate(candidates map[int]subscriptionNodeMatchCandidate, stableIdentityKey, link, contentHash, linkName string, sourceSort int) {
	if stableIdentityKey == "" || sourceSort <= 0 {
		return
	}
	var sourceSortMatches []models.Node
	for _, node := range matcher.unmatchedNodes(matcher.byStableIdentity[stableIdentityKey]) {
		if node.SourceSort == sourceSort {
			sourceSortMatches = append(sourceSortMatches, node)
		}
	}
	if len(sourceSortMatches) != 1 {
		return
	}
	matcher.addCandidates(candidates, sourceSortMatches, link, contentHash, linkName, sourceSort, subscriptionNodeStableIdentityScore)
}

func (matcher *subscriptionNodeMatcher) addSingleNodeFallbackCandidate(candidates map[int]subscriptionNodeMatchCandidate, link, contentHash, linkName string, sourceSort int) {
	if matcher.currentNodeCount != 1 || len(matcher.nodes) != 1 {
		return
	}
	unmatched := matcher.unmatchedNodes(matcher.nodes)
	if len(unmatched) != 1 {
		return
	}
	matcher.addCandidates(candidates, unmatched, link, contentHash, linkName, sourceSort, subscriptionNodeSingleNodeScore)
}

func (matcher *subscriptionNodeMatcher) unmatchedNodes(nodes []models.Node) []models.Node {
	if len(nodes) == 0 {
		return nil
	}
	unmatched := make([]models.Node, 0, len(nodes))
	for _, node := range nodes {
		if !matcher.matchedExistingIDs[node.ID] {
			unmatched = append(unmatched, node)
		}
	}
	return unmatched
}

func (matcher *subscriptionNodeMatcher) hashShrinking(contentHash string) bool {
	if contentHash == "" {
		return false
	}
	currentCount := matcher.currentHashCounts[contentHash]
	return currentCount > 0 && currentCount < len(matcher.byHash[contentHash])
}

func scoreSubscriptionNodeMatch(node models.Node, link, contentHash, linkName string, sourceSort int, hashShrinking bool) int {
	score := 0
	if link != "" && strings.TrimSpace(node.Link) == link {
		score += subscriptionNodeExactLinkScore
	}
	if contentHash != "" && node.ContentHash == contentHash {
		score += subscriptionNodeContentHashScore
		if linkName != "" && subscriptionNodeOriginalName(node) == linkName {
			score += 320
		}
		if sourceSort > 0 && node.SourceSort == sourceSort {
			score += 360
		}
		if !node.ShouldSyncNameFromLink() {
			score += 140
			if hashShrinking {
				score += 6000
			}
		}
	}
	if sourceSort > 0 && node.SourceSort == sourceSort {
		score += 360
	}
	return score
}

func bestSubscriptionNodeMatch(candidates map[int]subscriptionNodeMatchCandidate) (models.Node, bool) {
	var best subscriptionNodeMatchCandidate
	found := false
	bestScoreCount := 0
	for _, candidate := range candidates {
		if !found || candidate.score > best.score {
			best = candidate
			found = true
			bestScoreCount = 1
			continue
		}
		if candidate.score == best.score {
			bestScoreCount++
			if candidate.node.ID < best.node.ID {
				best = candidate
			}
		}
	}
	if !found || bestScoreCount != 1 {
		return models.Node{}, false
	}
	return best.node, true
}

func subscriptionNodeIDsToDelete(existingNodeByID map[int]models.Node, matcher *subscriptionNodeMatcher) []int {
	nodeIDsToDelete := make([]int, 0)
	for nodeID := range existingNodeByID {
		if matcher == nil || !matcher.isMatched(nodeID) {
			nodeIDsToDelete = append(nodeIDsToDelete, nodeID)
		}
	}
	sort.Ints(nodeIDsToDelete)
	return nodeIDsToDelete
}

var subscriptionNodeStableIdentityFieldsByProtocol = map[string][]string{
	"vless": {
		"Server",
		"Port",
		"Uuid",
		"Query.Type",
		"Query.Security",
		"Query.Flow",
		"Query.Sni",
		"Query.Pbk",
		"Query.Sid",
	},
	"trojan": {
		"Hostname",
		"Port",
		"Password",
		"Query.Type",
		"Query.Sni",
		"Query.Alpn",
	},
	"ss": {
		"Server",
		"Port",
		"Param.Cipher",
		"Param.Password",
		"Plugin",
	},
	"vmess": {
		"Add",
		"Port",
		"Id",
		"Aid",
		"Scy",
		"Net",
		"Tls",
		"Sni",
		"Host",
		"Path",
	},
}

var subscriptionNodeStableIdentityRequiredFieldsByProtocol = map[string][]string{
	"vless":  {"Uuid"},
	"trojan": {"Password"},
	"ss":     {"Param.Cipher", "Param.Password"},
	"vmess":  {"Id"},
}

func subscriptionNodeStableIdentityKey(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}

	protoObj, protoName, err := protocol.DecodeProtocolObject(link)
	if err != nil || protoObj == nil {
		return ""
	}
	identity, err := protocol.ExtractLinkIdentity(link)
	if err != nil {
		return ""
	}
	protoName = strings.ToLower(strings.TrimSpace(firstNonEmpty(identity.Protocol, protoName)))
	fieldPaths := subscriptionNodeStableIdentityFieldsByProtocol[protoName]
	if len(fieldPaths) == 0 || !subscriptionNodeHasRequiredStableIdentityFields(protoObj, protoName) {
		return ""
	}

	host := firstSubscriptionNodeProtocolField(protoObj, "Server", "Hostname", "Add", "Host")
	if host == "" {
		host = identity.Host
	}
	port := firstSubscriptionNodeProtocolField(protoObj, "Port")
	if port == "" {
		port = identity.Port
	}

	parts := []string{
		"protocol=" + protoName,
		"host=" + strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]")),
		"port=" + strings.TrimSpace(port),
	}
	for _, fieldPath := range fieldPaths {
		if value, ok := subscriptionNodeProtocolFieldValue(protoObj, fieldPath, true); ok {
			parts = append(parts, fieldPath+"="+value)
		}
	}
	return strings.Join(parts, "|")
}

func buildCurrentSubscriptionStableIdentityCounts(proxys []protocol.Proxy) map[string]int {
	counts := make(map[string]int)
	for _, proxy := range proxys {
		link := GenerateProxyLink(proxy)
		if link == "" {
			continue
		}
		if key := subscriptionNodeStableIdentityKey(link); key != "" {
			counts[key]++
		}
	}
	return counts
}

func subscriptionNodeHasRequiredStableIdentityFields(protoObj any, protoName string) bool {
	for _, fieldPath := range subscriptionNodeStableIdentityRequiredFieldsByProtocol[protoName] {
		if _, ok := subscriptionNodeProtocolFieldValue(protoObj, fieldPath, false); !ok {
			return false
		}
	}
	return true
}

func firstSubscriptionNodeProtocolField(protoObj any, fieldPaths ...string) string {
	for _, fieldPath := range fieldPaths {
		if value, ok := subscriptionNodeProtocolFieldValue(protoObj, fieldPath, false); ok {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func subscriptionNodeProtocolFieldValue(protoObj any, fieldPath string, includeZero bool) (string, bool) {
	v := reflect.ValueOf(protoObj)
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return "", false
		}
		v = v.Elem()
	}
	for _, part := range strings.Split(fieldPath, ".") {
		for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
			if v.IsNil() {
				return "", false
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return "", false
		}
		v = v.FieldByName(part)
		if !v.IsValid() {
			return "", false
		}
	}
	return subscriptionNodeStableValue(v, includeZero)
}

func subscriptionNodeStableValue(v reflect.Value, includeZero bool) (string, bool) {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return "", false
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.String:
		value := strings.TrimSpace(v.String())
		return value, includeZero || value != ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Int() == 0 && !includeZero {
			return "", false
		}
		return fmt.Sprintf("%d", v.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v.Uint() == 0 && !includeZero {
			return "", false
		}
		return fmt.Sprintf("%d", v.Uint()), true
	case reflect.Bool:
		if !v.Bool() && !includeZero {
			return "", false
		}
		return fmt.Sprintf("%t", v.Bool()), true
	case reflect.Slice, reflect.Array:
		if v.Len() == 0 && !includeZero {
			return "", false
		}
		values := make([]string, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			value, ok := subscriptionNodeStableValue(v.Index(i), includeZero)
			if ok {
				values = append(values, value)
			}
		}
		if len(values) == 0 && !includeZero {
			return "", false
		}
		sort.Strings(values)
		return strings.Join(values, ","), true
	case reflect.Map, reflect.Struct:
		if v.Kind() == reflect.Map && v.Len() == 0 && !includeZero {
			return "", false
		}
		value, err := json.Marshal(v.Interface())
		if err != nil || len(value) == 0 || (!includeZero && string(value) == "{}") {
			return "", false
		}
		return string(value), true
	default:
		return "", false
	}
}
