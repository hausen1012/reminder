package services

// chunkIDs 将 ID 切片按 size 分批，避免 SQLite 的 too many SQL variables 错误。
func chunkIDs(ids []uint, size int) [][]uint {
	if len(ids) == 0 {
		return nil
	}
	var chunks [][]uint
	for i := 0; i < len(ids); i += size {
		end := i + size
		if end > len(ids) {
			end = len(ids)
		}
		chunks = append(chunks, ids[i:end])
	}
	return chunks
}