// repo 集中服务层共用的数据访问片段：按 ID 取单条、多对多关联的读写。
package services

import (
	"errors"

	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// firstByID 按主键取一条记录；不存在时返回带业务文案的 404。
func firstByID[T any](db *gorm.DB, id any, notFound string) (*T, error) {
	var row T
	if err := db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, middleware.NewAppError(middleware.CodeNotFound, notFound)
		}
		return nil, err
	}
	return &row, nil
}

// joinIDs 取某个主体在关联表里的目标 ID 列表，按目标 ID 升序。
func joinIDs(db *gorm.DB, model any, ownerCol string, ownerID uint, targetCol string) ([]uint, error) {
	var ids []uint
	if err := db.Model(model).
		Where(ownerCol+" = ?", ownerID).
		Order(targetCol+" ASC").
		Pluck(targetCol, &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// joinIDsBatch 一次取多个主体在关联表里的目标 ID，避免列表接口 N+1 查询。
func joinIDsBatch(db *gorm.DB, model any, ownerCol, targetCol string, ownerIDs []uint) (map[uint][]uint, error) {
	out := make(map[uint][]uint, len(ownerIDs))
	if len(ownerIDs) == 0 {
		return out, nil
	}
	type pair struct {
		Owner  uint `gorm:"column:owner_id"`
		Target uint `gorm:"column:target_id"`
	}
	var rows []pair
	if err := db.Model(model).
		Select(ownerCol+" AS owner_id, "+targetCol+" AS target_id").
		Where(ownerCol+" IN ?", ownerIDs).
		Order(ownerCol + " ASC, " + targetCol + " ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.Owner] = append(out[row.Owner], row.Target)
	}
	return out, nil
}

// replaceJoinRows 在事务内整体替换某个主体的关联行；0 与重复 ID 会被丢弃。
func replaceJoinRows[T any](tx *gorm.DB, model any, ownerCol string, ownerID uint, ids []uint, build func(ownerID, targetID uint) T) error {
	if err := tx.Where(ownerCol+" = ?", ownerID).Delete(model).Error; err != nil {
		return err
	}
	rows := make([]T, 0, len(ids))
	seen := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		rows = append(rows, build(ownerID, id))
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

// reminderChannelIDs 返回提醒绑定的通道 ID。
func reminderChannelIDs(db *gorm.DB, reminderID uint) ([]uint, error) {
	return joinIDs(db, &models.ReminderChannel{}, "reminder_id", reminderID, "channel_id")
}

// loadChannelsByIDs 按 ID 加载通道；空列表返回空结果。
func loadChannelsByIDs(db *gorm.DB, ids []uint) ([]*models.Channel, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []models.Channel
	if err := db.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*models.Channel, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

// loadReminderChannels 加载提醒绑定的通道；未绑定时返回空切片。
func loadReminderChannels(db *gorm.DB, reminderID uint) ([]*models.Channel, error) {
	ids, err := reminderChannelIDs(db, reminderID)
	if err != nil {
		return nil, err
	}
	return loadChannelsByIDs(db, ids)
}

// tokenByChain 取某条确认链上的确认令牌：令牌挂在链的首条日志上，
// 后续轮次的日志没有对应 token 行，因此按 chain_id 关联查询。
func tokenByChain(db *gorm.DB, chainID string) (*models.ConfirmToken, error) {
	var tok models.ConfirmToken
	err := db.Joins("JOIN delivery_logs ON delivery_logs.id = confirm_tokens.delivery_log_id").
		Where("delivery_logs.confirm_chain_id = ?", chainID).
		First(&tok).Error
	if err != nil {
		return nil, err
	}
	return &tok, nil
}
