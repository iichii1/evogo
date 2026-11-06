package instance_model

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Instance struct {
	Id               string    `json:"id" gorm:"type:uuid;primaryKey"`
	Name             string    `json:"name"`
	Token            string    `json:"token" gorm:"unique"`
	Webhook          string    `json:"webhook"`
	RabbitmqEnable   string    `json:"rabbitmqEnable"`
	WebSocketEnable  string    `json:"websocketEnable"`
	NatsEnable       string    `json:"natsEnable"`
	Jid              string    `json:"jid" gorm:"column:jid"`
	Qrcode           string    `json:"qrcode" gorm:"type:text"`
	Connected        bool      `json:"connected"`
	Expiration       int64     `json:"expiration"`
	DisconnectReason string    `json:"disconnect_reason"`
	Events           string    `json:"events"`
	OsName           string    `json:"os_name"`
	Proxy            string    `json:"proxy"`
	ClientName       string    `json:"client_name"`
	CreatedAt        time.Time `json:"createdAt" gorm:"autoCreateTime"`

	// Advanced Settings
	AlwaysOnline  bool   `json:"alwaysOnline" gorm:"default:false"`
	RejectCall    bool   `json:"rejectCall" gorm:"default:false"`
	MsgRejectCall string `json:"msgRejectCall" gorm:"default:''"`
	ReadMessages  bool   `json:"readMessages" gorm:"default:false"`
	IgnoreGroups  bool   `json:"ignoreGroups" gorm:"default:false"`
	IgnoreStatus  bool   `json:"ignoreStatus" gorm:"default:false"`
}

// String identifica a instância em logs sem os segredos: token, QR, webhook, proxy e número
// ficam de fora, inclusive em quem imprimir a estrutura inteira com %v ou %+v.
func (i Instance) String() string {
	return fmt.Sprintf("Instance{Id:%s Name:%s Connected:%t}", i.Id, i.Name, i.Connected)
}

// AdvancedSettings representa as configurações avançadas de uma instância
type AdvancedSettings struct {
	AlwaysOnline  bool   `json:"alwaysOnline"`
	RejectCall    bool   `json:"rejectCall"`
	MsgRejectCall string `json:"msgRejectCall"`
	ReadMessages  bool   `json:"readMessages"`
	IgnoreGroups  bool   `json:"ignoreGroups"`
	IgnoreStatus  bool   `json:"ignoreStatus"`
}

// AdvancedSettingsPatch é o corpo aceito na atualização das configurações
// avançadas. Campo ausente significa "manter o valor atual": com os campos
// simples, enviar só {"alwaysOnline": true} zerava todos os outros (#111).
type AdvancedSettingsPatch struct {
	AlwaysOnline  *bool   `json:"alwaysOnline"`
	RejectCall    *bool   `json:"rejectCall"`
	MsgRejectCall *string `json:"msgRejectCall"`
	ReadMessages  *bool   `json:"readMessages"`
	IgnoreGroups  *bool   `json:"ignoreGroups"`
	IgnoreStatus  *bool   `json:"ignoreStatus"`
}

// Apply copia para s apenas os campos presentes no patch.
func (s *AdvancedSettings) Apply(p AdvancedSettingsPatch) {
	if p.AlwaysOnline != nil {
		s.AlwaysOnline = *p.AlwaysOnline
	}
	if p.RejectCall != nil {
		s.RejectCall = *p.RejectCall
	}
	if p.MsgRejectCall != nil {
		s.MsgRejectCall = *p.MsgRejectCall
	}
	if p.ReadMessages != nil {
		s.ReadMessages = *p.ReadMessages
	}
	if p.IgnoreGroups != nil {
		s.IgnoreGroups = *p.IgnoreGroups
	}
	if p.IgnoreStatus != nil {
		s.IgnoreStatus = *p.IgnoreStatus
	}
}

func (m *Instance) BeforeCreate(tx *gorm.DB) (err error) {
	if m.Id == "" {
		m.Id = uuid.New().String()
	}
	return
}
