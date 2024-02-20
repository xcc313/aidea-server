package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mylxsw/aidea-server/pkg/misc"
	"github.com/mylxsw/aidea-server/pkg/repo/model"
	"strings"
	"time"

	"github.com/mylxsw/aidea-server/config"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/eloquent"
	"github.com/mylxsw/eloquent/query"
	"github.com/mylxsw/go-utils/array"
	"github.com/mylxsw/go-utils/must"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/guregu/null.v3"
)

var (
	ErrUserInvalidCredentials = errors.New("用户名或密码错误")
	ErrUserAccountDisabled    = errors.New("账号已被注销")
	ErrUserExists             = errors.New("用户已存在")
)

const (
	UserStatusActive  = "active"
	UserStatusDeleted = "deleted"
)

const (
	// UserTypeNormal 普通用户
	UserTypeNormal = 0
	// UserTypeInternal 内部用户
	UserTypeInternal = 1
	// UserTypeTester 测试用户
	UserTypeTester = 2
	// UserTypeExtraPermission 例外用户
	UserTypeExtraPermission = 3
)

type UserRepo struct {
	db   *sql.DB
	conf *config.Config
}

func NewUserRepo(db *sql.DB, conf *config.Config) *UserRepo {
	return &UserRepo{db: db, conf: conf}
}

// GetUserByInviteCode 根据邀请码获取用户信息
func (repo *UserRepo) GetUserByInviteCode(ctx context.Context, code string) (*model.Users, error) {
	user, err := model.NewUsersModel(repo.db).First(ctx, query.Builder().Where(model.FieldUsersInviteCode, code))
	if err != nil {
		if errors.Is(err, query.ErrNoResult) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	if user.Status.ValueOrZero() == UserStatusDeleted {
		return nil, ErrUserAccountDisabled
	}

	ret := user.ToUsers()
	return &ret, nil
}

// UpdateUserInviteBy 更新用户的邀请人信息
func (repo *UserRepo) UpdateUserInviteBy(ctx context.Context, userId int64, invitedByUserId int64) error {
	_, err := model.NewUsersModel(repo.db).Update(ctx, query.Builder().Where(model.FieldUsersId, userId), model.UsersN{
		InvitedBy: null.IntFrom(invitedByUserId),
	})

	return err
}

// GenerateInviteCode 为用户生成邀请码
func (repo *UserRepo) GenerateInviteCode(ctx context.Context, userId int64) error {
	_, err := model.NewUsersModel(repo.db).Update(ctx, query.Builder().Where(model.FieldUsersId, userId), model.UsersN{
		InviteCode: null.StringFrom(misc.HashID(userId)),
	})

	return err
}

// GetUserByID 根据用户ID获取用户信息
func (repo *UserRepo) GetUserByID(ctx context.Context, userID int64) (*model.Users, error) {
	user, err := model.NewUsersModel(repo.db).First(ctx, query.Builder().Where(model.FieldUsersId, userID))
	if err != nil {
		if errors.Is(err, query.ErrNoResult) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	if user.Status.ValueOrZero() == UserStatusDeleted {
		return nil, ErrUserAccountDisabled
	}

	ret := user.ToUsers()
	return &ret, nil
}

// GetUserByPhone 根据用户手机号获取用户信息
func (repo *UserRepo) GetUserByPhone(ctx context.Context, phone string) (*model.Users, error) {
	user, err := model.NewUsersModel(repo.db).First(ctx, query.Builder().Where(model.FieldUsersPhone, phone))
	if err != nil {
		if errors.Is(err, query.ErrNoResult) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	if user.Status.ValueOrZero() == UserStatusDeleted {
		return nil, ErrUserAccountDisabled
	}

	ret := user.ToUsers()
	return &ret, nil
}

// GetUserByEmail 根据用户邮箱地址获取用户信息
func (repo *UserRepo) GetUserByEmail(ctx context.Context, username string) (*model.Users, error) {
	user, err := model.NewUsersModel(repo.db).First(ctx, query.Builder().Where(model.FieldUsersEmail, username))
	if err != nil {
		if errors.Is(err, query.ErrNoResult) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	if user.Status.ValueOrZero() == UserStatusDeleted {
		return nil, ErrUserAccountDisabled
	}

	ret := user.ToUsers()
	return &ret, nil
}

// VerifyPassword 验证用户密码
func (repo *UserRepo) VerifyPassword(ctx context.Context, userID int64, password string) error {
	user, err := model.NewUsersModel(repo.db).First(ctx, query.Builder().Where(model.FieldUsersId, userID))
	if err != nil {
		if errors.Is(err, query.ErrNoResult) {
			return ErrNotFound
		}

		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password.ValueOrZero()), []byte(password)); err != nil {
		return ErrUserInvalidCredentials
	}

	return nil
}

// UpdateStatus 更新用户状态
func (repo *UserRepo) UpdateStatus(ctx context.Context, userID int64, status string) error {
	_, err := model.NewUsersModel(repo.db).Update(ctx, query.Builder().Where(model.FieldUsersId, userID), model.UsersN{
		Status: null.StringFrom(status),
	})

	return err
}

// UpdatePassword 更新用户密码
func (repo *UserRepo) UpdatePassword(ctx context.Context, userID int64, password string) error {
	encryptedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = model.NewUsersModel(repo.db).Update(ctx, query.Builder().Where(model.FieldUsersId, userID), model.UsersN{
		Password: null.StringFrom(string(encryptedPassword)),
	})

	return err
}

// UpdateAvatarURL 更新用户头像
func (repo *UserRepo) UpdateAvatarURL(ctx context.Context, userID int64, avatarURL string) error {
	_, err := model.NewUsersModel(repo.db).Update(ctx, query.Builder().Where(model.FieldUsersId, userID), model.UsersN{
		Avatar: null.StringFrom(avatarURL),
	})

	return err
}

// UpdateRealname 更新用户真实姓名
func (repo *UserRepo) UpdateRealname(ctx context.Context, userID int64, realname string) error {
	_, err := model.NewUsersModel(repo.db).Update(ctx, query.Builder().Where(model.FieldUsersId, userID), model.UsersN{
		Realname: null.StringFrom(realname),
	})

	return err
}

// SignUpPhone 使用手机号注册用户
func (repo *UserRepo) SignUpPhone(ctx context.Context, username string, password string, realname string) (user *model.Users, eventID int64, err error) {
	if err = eloquent.Transaction(repo.db, func(tx query.Database) error {
		q := query.Builder().Where(model.FieldUsersPhone, username)
		matchedCount, err := model.NewUsersModel(tx).Count(ctx, q)
		if err != nil {
			return err
		}

		if matchedCount > 0 {
			return ErrUserExists
		}

		user = &model.Users{
			Phone:    username,
			Realname: realname,
			Status:   UserStatusActive,
		}

		if password != "" {
			encryptedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				return err
			}

			user.Password = string(encryptedPassword)
		}

		id, err := model.NewUsersModel(tx).Save(ctx, user.ToUsersN(
			model.FieldUsersPhone,
			model.FieldUsersPassword,
			model.FieldUsersRealname,
			model.FieldUsersStatus,
		))
		if err != nil {
			return err
		}
		user.Id = id

		if eventID, err = model.NewEventsModel(tx).Save(ctx, model.EventsN{
			EventType: null.StringFrom(EventTypeUserCreated),
			Payload:   null.StringFrom(string(must.Must(json.Marshal(UserCreatedEvent{UserID: user.Id, From: UserCreatedEventSourcePhone})))),
			Status:    null.StringFrom(EventStatusWaiting),
		}); err != nil {
			log.With(user).Errorf("create event failed: %s", err)
			return err
		}

		return nil
	}); err != nil {
		return nil, 0, err
	}

	return user, eventID, nil
}

// SignUpEmail 使用邮箱注册用户
func (repo *UserRepo) SignUpEmail(ctx context.Context, username string, password string, realname string) (user *model.Users, eventID int64, err error) {
	if err = eloquent.Transaction(repo.db, func(tx query.Database) error {
		q := query.Builder().Where(model.FieldUsersEmail, username)
		matchedCount, err := model.NewUsersModel(tx).Count(ctx, q)
		if err != nil {
			return err
		}

		if matchedCount > 0 {
			return ErrUserExists
		}

		user = &model.Users{
			Email:    username,
			Realname: realname,
			Status:   UserStatusActive,
		}

		if password != "" {
			encryptedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			user.Password = string(encryptedPassword)
		}

		id, err := model.NewUsersModel(tx).Save(ctx, user.ToUsersN(
			model.FieldUsersEmail,
			model.FieldUsersPassword,
			model.FieldUsersRealname,
			model.FieldUsersStatus,
		))
		if err != nil {
			return err
		}
		user.Id = id

		if eventID, err = model.NewEventsModel(tx).Save(ctx, model.EventsN{
			EventType: null.StringFrom(EventTypeUserCreated),
			Payload:   null.StringFrom(string(must.Must(json.Marshal(UserCreatedEvent{UserID: user.Id, From: UserCreatedEventSourceEmail})))),
			Status:    null.StringFrom(EventStatusWaiting),
		}); err != nil {
			log.With(user).Errorf("create event failed: %s", err)
			return err
		}

		return nil
	}); err != nil {
		return nil, 0, err
	}

	return user, eventID, nil
}

// SignIn 用户登录
func (repo *UserRepo) SignIn(ctx context.Context, emailOrPhone, password string) (*model.Users, error) {
	q := query.Builder().Where(model.FieldUsersEmail, emailOrPhone).OrWhere(model.FieldUsersPhone, emailOrPhone)
	user, err := model.NewUsersModel(repo.db).First(ctx, q)
	if err != nil {
		if errors.Is(err, query.ErrNoResult) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	if user.Status.ValueOrZero() == UserStatusDeleted {
		return nil, ErrUserAccountDisabled
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password.ValueOrZero()), []byte(password)); err != nil {
		return nil, ErrUserInvalidCredentials
	}

	u := user.ToUsers()
	return &u, nil
}

// WeChatIsBond 微信是否已经绑定
func (repo *UserRepo) WeChatIsBond(ctx context.Context, unionID string) (bool, error) {
	q := query.Builder().Where(model.FieldUsersUnionId, unionID)
	matchedCount, err := model.NewUsersModel(repo.db).Count(ctx, q)
	if err != nil {
		return false, err
	}

	return matchedCount > 0, nil
}

// BindWeChat 绑定微信
func (repo *UserRepo) BindWeChat(ctx context.Context, userID int64, unionID string, nickname string, avatarURL string) error {
	return eloquent.Transaction(repo.db, func(tx query.Database) error {
		q := query.Builder().Where(model.FieldUsersId, userID)
		user, err := model.NewUsersModel(tx).First(ctx, q)
		if err != nil {
			return ErrNotFound
		}

		if user.UnionId.ValueOrZero() == unionID {
			return nil
		}

		if user.UnionId.ValueOrZero() != "" && user.UnionId.ValueOrZero() != unionID {
			return errors.New("当前账号已绑定其他微信")
		}

		bond, err := repo.WeChatIsBond(ctx, unionID)
		if err != nil {
			return err
		}

		if bond {
			return errors.New("当前微信已绑定其他账号")
		}

		if user.Realname.ValueOrZero() == "" {
			user.Realname = null.StringFrom(nickname)
		}

		if user.Avatar.ValueOrZero() == "" {
			user.Avatar = null.StringFrom(avatarURL)
		}

		user.UnionId = null.StringFrom(unionID)
		return user.Save(ctx, model.FieldUsersUnionId, model.FieldUsersRealname, model.FieldUsersAvatar)
	})
}

// WeChatSignIn 微信登录
func (repo *UserRepo) WeChatSignIn(ctx context.Context, unionID string, nickname string, avatarURL string) (user *model.Users, eventID int64, err error) {
	err = eloquent.Transaction(repo.db, func(tx query.Database) error {
		q := query.Builder().
			Where(model.FieldUsersUnionId, unionID)
		matched, err := model.NewUsersModel(tx).Get(ctx, q)
		if err != nil {
			return err
		}

		// 如果没有匹配的用户，那么创建一个新用户
		if len(matched) == 0 {
			user = &model.Users{
				UnionId:  unionID,
				Realname: nickname,
				Avatar:   avatarURL,
				Status:   UserStatusActive,
			}

			id, err := model.NewUsersModel(tx).Save(ctx, user.ToUsersN(
				model.FieldUsersUnionId,
				model.FieldUsersAvatar,
				model.FieldUsersRealname,
				model.FieldUsersStatus,
			))
			if err != nil {
				return err
			}
			user.Id = id

			if eventID, err = model.NewEventsModel(tx).Save(ctx, model.EventsN{
				EventType: null.StringFrom(EventTypeUserCreated),
				Payload:   null.StringFrom(string(must.Must(json.Marshal(UserCreatedEvent{UserID: user.Id, From: "wechat"})))),
				Status:    null.StringFrom(EventStatusWaiting),
			}); err != nil {
				log.With(user).Errorf("create event failed: %s", err)
				return err
			}

			return nil
		}

		// 如果只有一个匹配的用户，那么直接返回
		if len(matched) == 1 {
			matched[0].UnionId = null.StringFrom(unionID)
			if err := matched[0].Save(ctx, model.FieldUsersUnionId); err != nil {
				log.With(matched[0]).Errorf("update user failed: %s", err)
			}

			matchedUser := matched[0].ToUsers()
			user = &matchedUser
			return nil
		}

		// 如果有多个匹配的用户
		loginUser := array.Filter(matched, func(user model.UsersN, _ int) bool { return user.UnionId.ValueOrZero() == unionID })
		if len(loginUser) != 1 {
			return errors.New("apple login failed: multiple users matched")
		}

		matchedUser := loginUser[0].ToUsers()
		user = &matchedUser
		return nil
	})

	if user != nil && user.Status == UserStatusDeleted {
		return nil, 0, ErrUserAccountDisabled
	}

	return user, eventID, err
}

// AppleSignIn Apple 登录
func (repo *UserRepo) AppleSignIn(
	ctx context.Context,
	appleUID string,
	email string,
	isPrivateEmail bool,
	familyName, givenName string,
) (user *model.Users, eventID int64, err error) {
	err = eloquent.Transaction(repo.db, func(tx query.Database) error {
		q := query.Builder().
			Where(model.FieldUsersAppleUid, appleUID).
			OrWhere(model.FieldUsersEmail, email)
		matched, err := model.NewUsersModel(tx).Get(ctx, q)
		if err != nil {
			return err
		}

		// 如果没有匹配的用户，那么创建一个新用户
		if len(matched) == 0 {
			user = &model.Users{
				AppleUid: appleUID,
				Email:    email,
				Realname: strings.TrimSpace(givenName + " " + familyName),
				Status:   UserStatusActive,
			}

			id, err := model.NewUsersModel(tx).Save(ctx, user.ToUsersN(
				model.FieldUsersAppleUid,
				model.FieldUsersEmail,
				model.FieldUsersRealname,
				model.FieldUsersStatus,
			))
			if err != nil {
				return err
			}
			user.Id = id

			if eventID, err = model.NewEventsModel(tx).Save(ctx, model.EventsN{
				EventType: null.StringFrom(EventTypeUserCreated),
				Payload:   null.StringFrom(string(must.Must(json.Marshal(UserCreatedEvent{UserID: user.Id, From: "apple"})))),
				Status:    null.StringFrom(EventStatusWaiting),
			}); err != nil {
				log.With(user).Errorf("create event failed: %s", err)
				return err
			}

			return nil
		}

		// 如果只有一个匹配的用户，那么直接返回
		if len(matched) == 1 {
			matched[0].AppleUid = null.StringFrom(appleUID)
			if err := matched[0].Save(ctx, model.FieldUsersAppleUid); err != nil {
				log.With(matched[0]).Errorf("update user failed: %s", err)
			}

			matchedUser := matched[0].ToUsers()
			user = &matchedUser
			return nil
		}

		// 如果有多个匹配的用户
		appleLoginUser := array.Filter(matched, func(user model.UsersN, _ int) bool { return user.AppleUid.ValueOrZero() == appleUID })
		if len(appleLoginUser) != 1 {
			return errors.New("apple login failed: multiple users matched")
		}

		matchedUser := appleLoginUser[0].ToUsers()
		user = &matchedUser
		return nil
	})

	if user != nil && user.Status == UserStatusDeleted {
		return nil, 0, ErrUserAccountDisabled
	}

	return user, eventID, err
}

func (repo *UserRepo) BindPhone(ctx context.Context, userID int64, phone string, sendEvent bool) (eventID int64, err error) {
	q := query.Builder().Where(model.FieldUsersId, userID)
	err = eloquent.Transaction(repo.db, func(tx query.Database) error {
		if _, err := model.NewUsersModel(tx).Update(
			ctx,
			q,
			model.UsersN{
				Phone: null.StringFrom(phone),
			},
			model.FieldUsersPhone,
		); err != nil {
			return fmt.Errorf("update user's phone failed: %w", err)
		}

		if !sendEvent {
			return nil
		}

		if eventID, err = model.NewEventsModel(tx).Save(ctx, model.EventsN{
			EventType: null.StringFrom(EventTypeUserPhoneBound),
			Payload:   null.StringFrom(string(must.Must(json.Marshal(UserBindEvent{UserID: userID, Phone: phone})))),
			Status:    null.StringFrom(EventStatusWaiting),
		}); err != nil {
			log.WithFields(log.Fields{
				"user_id": userID,
				"phone":   phone,
			}).Errorf("create event failed: %s", err)
			return err
		}

		return nil
	})

	return
}

// UserCustomConfig 用户自定义配置
type UserCustomConfig struct {
	// HomeModels 主页显示的模型
	HomeModels   []string      `json:"home_models,omitempty"`
	HomeModelsV2 []HomeModelV2 `json:"home_models_v2,omitempty"`
}

type HomeModelV2 struct {
	Type          string `json:"type"`
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	SupportVision bool   `json:"support_vision,omitempty"`
}

// CustomConfig 查询用户自定义配置
func (repo *UserRepo) CustomConfig(ctx context.Context, userID int64) (*UserCustomConfig, error) {
	user, err := model.NewUserCustomModel(repo.db).First(ctx, query.Builder().Where(model.FieldUserCustomUserId, userID))
	if err != nil && !errors.Is(err, query.ErrNoResult) {
		return nil, fmt.Errorf("查询用户自定义配置失败：%w", err)
	}

	var configData string
	if errors.Is(err, query.ErrNoResult) {
		configData = "{}"
	} else {
		configData = user.Config.ValueOrZero()
	}

	var customConfig UserCustomConfig
	if err := json.Unmarshal([]byte(configData), &customConfig); err != nil {
		return nil, fmt.Errorf("解析用户 %d 自定义配置失败：%w", userID, err)
	}

	return &customConfig, nil
}

// UpdateCustomConfig 更新用户自定义配置
func (repo *UserRepo) UpdateCustomConfig(ctx context.Context, userID int64, conf UserCustomConfig) error {
	configData, err := json.Marshal(conf)
	if err != nil {
		return fmt.Errorf("序列化用户 %d 自定义配置失败：%w", userID, err)
	}

	return eloquent.Transaction(repo.db, func(tx query.Database) error {
		q := query.Builder().Where(model.FieldUserCustomUserId, userID)

		cus, err := model.NewUserCustomModel(tx).First(ctx, q)
		if err != nil && !errors.Is(err, query.ErrNoResult) {
			return fmt.Errorf("查询用户自定义配置失败：%w", err)
		}

		if errors.Is(err, query.ErrNoResult) {
			_, err = model.NewUserCustomModel(tx).Save(ctx, model.UserCustomN{
				UserId: null.IntFrom(userID),
				Config: null.StringFrom(string(configData)),
			})
			return err
		}

		_, err = model.NewUserCustomModel(tx).UpdateById(
			ctx,
			cus.Id.ValueOrZero(),
			model.UserCustomN{Config: null.StringFrom(string(configData))},
			model.FieldUserCustomConfig,
		)

		return err
	})
}

const (
	UserApiKeyStatusDisabled = 2
	UserAPiKeyStatusActive   = 1
)

// GetUserByAPIKey 根据 API Token 获取用户信息
func (repo *UserRepo) GetUserByAPIKey(ctx context.Context, token string) (*model.Users, error) {
	key, err := model.NewUserApiKeyModel(repo.db).First(ctx, query.Builder().Where(model.FieldUserApiKeyToken, token))
	if err != nil {
		if errors.Is(err, query.ErrNoResult) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	apiKey := key.ToUserApiKey()
	if apiKey.Status == UserApiKeyStatusDisabled {
		return nil, ErrNotFound
	}

	if !apiKey.ValidBefore.IsZero() && apiKey.ValidBefore.Before(time.Now()) {
		return nil, ErrNotFound
	}

	return repo.GetUserByID(ctx, apiKey.UserId)
}

// GetAPIKeys 获取用户的 API Keys
func (repo *UserRepo) GetAPIKeys(ctx context.Context, userID int64) ([]model.UserApiKey, error) {
	q := query.Builder().Where(model.FieldUserApiKeyUserId, userID).
		Where(model.FieldUserApiKeyStatus, UserAPiKeyStatusActive)
	keys, err := model.NewUserApiKeyModel(repo.db).Get(ctx, q)
	if err != nil {
		return nil, err
	}

	return array.Map(keys, func(key model.UserApiKeyN, _ int) model.UserApiKey {
		item := key.ToUserApiKey()
		item.Token = misc.MaskStr(item.Token, 6)
		return item
	}), nil
}

// GetAPIKey 获取用户的 API Key
func (repo *UserRepo) GetAPIKey(ctx context.Context, userID int64, keyID int64) (*model.UserApiKey, error) {
	key, err := model.NewUserApiKeyModel(repo.db).First(ctx, query.Builder().
		Where(model.FieldUserApiKeyUserId, userID).
		Where(model.FieldUserApiKeyId, keyID).
		Where(model.FieldUserApiKeyStatus, UserAPiKeyStatusActive),
	)
	if err != nil {
		if errors.Is(err, query.ErrNoResult) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	ret := key.ToUserApiKey()
	return &ret, nil
}

// CreateAPIKey 创建一个 API Token
func (repo *UserRepo) CreateAPIKey(ctx context.Context, userID int64, name string, validBefore time.Time) (string, error) {
	key := model.UserApiKey{
		UserId:      userID,
		Name:        name,
		ValidBefore: validBefore,
		Status:      UserAPiKeyStatusActive,
		Token:       fmt.Sprintf("sk-%s", misc.GenerateAPIToken(name, userID)),
	}

	allows := []string{
		model.FieldUserApiKeyUserId,
		model.FieldUserApiKeyName,
		model.FieldUserApiKeyToken,
		model.FieldUserApiKeyStatus,
	}

	if !validBefore.IsZero() {
		allows = append(allows, model.FieldUserApiKeyValidBefore)
	}

	id, err := model.NewUserApiKeyModel(repo.db).Save(ctx, key.ToUserApiKeyN(allows...))
	if err != nil {
		return "", err
	}

	key.Id = id
	return key.Token, nil
}

// DeleteAPIKey 删除一个 API Key
func (repo *UserRepo) DeleteAPIKey(ctx context.Context, userID int64, keyID int64) error {
	//_, err := model.NewUserApiKeyModel(repo.db).Delete(ctx, query.Builder().
	//	Where(model.FieldUserApiKeyUserId, userID).
	//	Where(model.FieldUserApiKeyId, keyID),
	//)

	q := query.Builder().Where(model.FieldUserApiKeyUserId, userID).Where(model.FieldUserApiKeyId, keyID)
	update := query.KV{model.FieldUserApiKeyStatus: UserApiKeyStatusDisabled}

	_, err := model.NewUserApiKeyModel(repo.db).UpdateFields(ctx, update, q)
	return err
}
