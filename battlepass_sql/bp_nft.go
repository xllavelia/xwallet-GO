package battlepass_sql

import (
	"strconv"
	"strings"
)

// bpNft — NFT-награда: одиночная редкость, случайный пак (count > 1) или известный пак свежего дропа.
func bpNft(rarity string, count int) BPReward {
	title := strings.ToUpper(rarity[:1]) + rarity[1:]
	if count > 1 {
		return BPReward{Kind: "nft", Rarity: rarity, Count: count, Label: title, Display: strconv.Itoa(count) + " random " + title + " NFTs"}
	}
	return BPReward{Kind: "nft", Rarity: rarity, Count: 1, Label: title, Display: title + " NFT"}
}

func bpNftPack(code, name string) BPReward {
	return BPReward{Kind: "nft", Pack: code, Label: name, Display: name + " NFT pack"}
}

// NFT добавляются к существующим наградам уровня, не заменяя их.
var bpNftRewards = map[int]BPLevelRewards{
	3:  {Free: []BPReward{bpNft("common", 1)}, Pass: []BPReward{bpNft("uncommon", 1)}},
	8:  {Free: []BPReward{bpNft("uncommon", 1)}, Pass: []BPReward{bpNft("rare", 1)}},
	12: {Free: []BPReward{bpNft("common", 3)}, Pass: []BPReward{bpNft("uncommon", 3)}},
	17: {Free: []BPReward{bpNft("rare", 1)}, Pass: []BPReward{bpNft("epic", 1)}},
	23: {Free: []BPReward{bpNftPack("collector", "Collector Box")}, Pass: []BPReward{bpNftPack("vault", "Vault Box")}},
	33: {Free: []BPReward{bpNft("uncommon", 3)}, Pass: []BPReward{bpNft("rare", 3)}},
	38: {Free: []BPReward{bpNft("rare", 1)}, Pass: []BPReward{bpNft("epic", 1)}},
	44: {Free: []BPReward{bpNftPack("collector", "Collector Box")}, Pass: []BPReward{bpNftPack("vault", "Vault Box")}},
	50: {Free: []BPReward{bpNft("epic", 1)}, Pass: []BPReward{bpNft("mythic", 1)}},
}

func init() {
	for level, r := range bpNftRewards {
		BPPassRewards[level-1].Free = append(BPPassRewards[level-1].Free, r.Free...)
		BPPassRewards[level-1].Pass = append(BPPassRewards[level-1].Pass, r.Pass...)
	}
}
