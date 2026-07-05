package node

import (
	"strings"

	"sublink/models"
)

type subscriptionNodeMatcher struct {
	byLink            map[string][]models.Node
	byHash            map[string][]models.Node
	byHashName        map[string]map[string][]models.Node
	byHashSort        map[string]map[int][]models.Node
	currentHashCounts map[string]int
	matched           map[int]bool
}

type subscriptionNodeMatchCandidate struct {
	node  models.Node
	score int
}

func newSubscriptionNodeMatcher(nodes []models.Node, currentHashCounts map[string]int) *subscriptionNodeMatcher {
	matcher := &subscriptionNodeMatcher{
		byLink:            make(map[string][]models.Node),
		byHash:            make(map[string][]models.Node),
		byHashName:        make(map[string]map[string][]models.Node),
		byHashSort:        make(map[string]map[int][]models.Node),
		currentHashCounts: currentHashCounts,
		matched:           make(map[int]bool, len(nodes)),
	}
	for _, node := range nodes {
		if link := strings.TrimSpace(node.Link); link != "" {
			matcher.byLink[link] = append(matcher.byLink[link], node)
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
	return matcher.matched[id]
}

func (matcher *subscriptionNodeMatcher) match(link, contentHash, linkName string, sourceSort int) (models.Node, bool) {
	if matcher == nil {
		return models.Node{}, false
	}

	link = strings.TrimSpace(link)
	linkName = strings.TrimSpace(linkName)

	candidates := make(map[int]subscriptionNodeMatchCandidate)
	matcher.addCandidates(candidates, matcher.byLink[link], link, contentHash, linkName, sourceSort)
	if contentHash != "" {
		matcher.addCandidates(candidates, matcher.byHashName[contentHash][linkName], link, contentHash, linkName, sourceSort)
		if sourceSort > 0 {
			matcher.addCandidates(candidates, matcher.byHashSort[contentHash][sourceSort], link, contentHash, linkName, sourceSort)
		}
		matcher.addCandidates(candidates, matcher.byHash[contentHash], link, contentHash, linkName, sourceSort)
	}

	best, ok := bestSubscriptionNodeMatch(candidates)
	if !ok {
		return models.Node{}, false
	}
	matcher.matched[best.ID] = true
	return best, true
}

func (matcher *subscriptionNodeMatcher) addCandidates(candidates map[int]subscriptionNodeMatchCandidate, nodes []models.Node, link, contentHash, linkName string, sourceSort int) {
	hashShrinking := matcher.hashShrinking(contentHash)
	for _, node := range nodes {
		if matcher.matched[node.ID] {
			continue
		}
		score := scoreSubscriptionNodeMatch(node, link, contentHash, linkName, sourceSort, hashShrinking)
		if score <= 0 {
			continue
		}
		if existing, ok := candidates[node.ID]; !ok || score > existing.score {
			candidates[node.ID] = subscriptionNodeMatchCandidate{node: node, score: score}
		}
	}
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
		score += 1000
	}
	if contentHash != "" && node.ContentHash == contentHash {
		score += 100
		if linkName != "" && subscriptionNodeOriginalName(node) == linkName {
			score += 320
		}
		if sourceSort > 0 && node.SourceSort == sourceSort {
			score += 360
		}
		if !node.ShouldSyncNameFromLink() {
			score += 140
			if hashShrinking {
				score += 1000
			}
		}
	}
	return score
}

func bestSubscriptionNodeMatch(candidates map[int]subscriptionNodeMatchCandidate) (models.Node, bool) {
	var best subscriptionNodeMatchCandidate
	found := false
	for _, candidate := range candidates {
		if !found || candidate.score > best.score || (candidate.score == best.score && candidate.node.ID < best.node.ID) {
			best = candidate
			found = true
		}
	}
	return best.node, found
}
