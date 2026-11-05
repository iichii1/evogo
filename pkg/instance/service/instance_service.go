package instance_service

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/evolution-foundation/evolution-go/pkg/runtimestate"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
	event_types "github.com/evolution-foundation/evolution-go/pkg/internal/event_types"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type InstanceService interface {
	Create(data *CreateStruct) (*instance_model.Instance, error)
	Connect(data *ConnectStruct, instance *instance_model.Instance) (*instance_model.Instance, string, string, error)
	Reconnect(instance *instance_model.Instance) error
	Disconnect(instance *instance_model.Instance) (*instance_model.Instance, error)
	Logout(instance *instance_model.Instance) (*instance_model.Instance, error)
	Status(instance *instance_model.Instance) (*StatusStruct, error)
	GetQr(instance *instance_model.Instance) (*QrcodeStruct, error)
	Pair(data *PairStruct, instance *instance_model.Instance) (*PairReturnStruct, error)
	GetAll() ([]*instance_model.Instance, error)
	Info(instanceId string) (*instance_model.Instance, error)
	Delete(id string) error
	SetProxy(id string, proxyConfig *ProxyConfig) error
	SetProxyFromStruct(id string, data *SetProxyStruct) error
	RemoveProxy(id string) error
	ForceReconnect(instanceId string, number string) error
	GetInstanceByToken(token string) (*instance_model.Instance, error)
	GetLogs(instanceId string, startDate, endDate time.Time, level string, limit int) ([]logger_wrapper.LogEntry, error)
	GetAdvancedSettings(instanceId string) (*instance_model.AdvancedSettings, error)
	UpdateAdvancedSettings(instanceId string, settings *instance_model.AdvancedSettings) error
}

// ErrAlreadyAuthenticated indica que a instância já está pareada e logada, então
// não há o que parear. É uma situação de estado (409), não uma falha do servidor.
var ErrAlreadyAuthenticated = errors.New("instance is already authenticated")

type instances struct {
	instanceRepository instance_repository.InstanceRepository
	config             *config.Config
	clientPointer      *runtimestate.Map[string, *whatsmeow.Client]
	whatsmeowService   whatsmeow_service.WhatsmeowService
	loggerWrapper      *logger_wrapper.LoggerManager
}

type ProxyConfig struct {
	Protocol string `json:"protocol,omitempty"`
	Port     string `json:"port"`
	Password string `json:"password"`
	Username string `json:"username"`
	Host     string `json:"host"`
}

type CreateStruct struct {
	InstanceId       string                           `json:"instanceId"`
	Name             string                           `json:"name"`
	Token            string                           `json:"token"`
	Proxy            *ProxyConfig                     `json:"proxy"`
	AdvancedSettings *instance_model.AdvancedSettings `json:"advancedSettings"`
}

type ConnectStruct struct {
	WebhookUrl      string   `json:"webhookUrl"`
	Subscribe       []string `json:"subscribe"`
	Immediate       bool     `json:"immediate"`
	Phone           string   `json:"phone"`
	RabbitmqEnable  string   `json:"rabbitmqEnable"`
	WebSocketEnable string   `json:"websocketEnable"`
	NatsEnable      string   `json:"natsEnable"`
}

type StatusStruct struct {
	Connected bool
	LoggedIn  bool
	myJid     *types.JID
	Name      string
}

type QrcodeStruct struct {
	Qrcode string `json:"qrcode"`
	Code   string `json:"code"`
	// Passkey ceremony fields. Populated when the account requires a WebAuthn
	// passkey to finish linking (no QR to scan at that point). The manager uses
	// PasskeyStage to switch its UI and PasskeyOpenUrl for the
	// "Abrir WhatsApp Web" button that launches the passkey ceremony.
	PasskeyStage   string `json:"passkeyStage,omitempty"`
	PasskeyOpenURL string `json:"passkeyOpenUrl,omitempty"`
	PasskeyCode    string `json:"passkeyCode,omitempty"`
}

type PairStruct struct {
	Subscribe []string `json:"subscribe"`
	Phone     string   `json:"phone"`
}

type PairReturnStruct struct {
	PairingCode string
}

type SetProxyStruct struct {
	Protocol string `json:"protocol,omitempty"`
	Host     string `json:"host" validate:"required"`
	Port     string `json:"port" validate:"required"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type ForceReconnectStruct struct {
	Number string `json:"number"`
}

func (i *instances) ensureClientConnected(instanceId string) (*whatsmeow.Client, error) {
	logger := i.loggerWrapper.GetLogger(instanceId)
	client := i.clientPointer.Get(instanceId)
	logger.LogInfo("[%s] Checking client connection status - Client exists: %v", instanceId, client != nil)

	if client == nil {
		logger.LogInfo("[%s] No client found, attempting to start new instance", instanceId)
		err := i.whatsmeowService.StartInstance(instanceId)
		if err != nil {
			logger.LogError("[%s] Failed to start instance: %v", instanceId, err)
			return nil, errors.New("no active session found")
		}

		logger.LogInfo("[%s] Instance started, waiting 2 seconds...", instanceId)
		time.Sleep(2 * time.Second)

		client = i.clientPointer.Get(instanceId)
		logger.LogInfo("[%s] Checking new client - Exists: %v, Connected: %v",
			instanceId,
			client != nil,
			client != nil && client.IsConnected())

		if client == nil || !client.IsConnected() {
			logger.LogError("[%s] New client validation failed - Exists: %v, Connected: %v",
				instanceId,
				client != nil,
				client != nil && client.IsConnected())
			return nil, errors.New("no active session found")
		}
	} else if !client.IsConnected() {
		logger.LogError("[%s] Existing client is disconnected - Connected status: %v",
			instanceId,
			client.IsConnected())
		return nil, errors.New("client disconnected")
	}

	logger.LogInfo("[%s] Client successfully validated - Connected: %v", instanceId, client.IsConnected())
	return client, nil
}

func (i instances) Create(data *CreateStruct) (*instance_model.Instance, error) {
	if data.Proxy != nil {
		data.Proxy.Protocol = utils.NormalizeProxyProtocol(data.Proxy.Protocol, data.Proxy.Port)
	}

	proxyJson, err := json.Marshal(data.Proxy)
	if err != nil {
		return nil, err
	}

	findInstance, _ := i.instanceRepository.GetInstanceByName(data.Name)

	if findInstance != nil {
		return nil, fmt.Errorf("instance already exists")
	}

	instance := instance_model.Instance{
		Id:         data.InstanceId,
		Name:       data.Name,
		Token:      data.Token,
		OsName:     i.config.OsName,
		Proxy:      string(proxyJson),
		Connected:  false,
		ClientName: i.config.ClientName,
	}

	// Set advanced settings if provided
	if data.AdvancedSettings != nil {
		instance.AlwaysOnline = data.AdvancedSettings.AlwaysOnline
		instance.RejectCall = data.AdvancedSettings.RejectCall
		instance.MsgRejectCall = data.AdvancedSettings.MsgRejectCall
		instance.ReadMessages = data.AdvancedSettings.ReadMessages
		instance.IgnoreGroups = data.AdvancedSettings.IgnoreGroups
		instance.IgnoreStatus = data.AdvancedSettings.IgnoreStatus
	}

	createdInstance, err := i.instanceRepository.Create(instance)
	if err != nil {
		return nil, err
	}

	return createdInstance, nil
}

// applyConnectSettings aplica ao registro da instância o que veio no connect e
// devolve a lista de eventos assinados.
//
// Campo omitido mantém o valor atual. Antes o connect sobrescrevia tudo com o
// que veio no corpo, então um connect sem corpo zerava webhook, RabbitMQ, NATS e
// WebSocket e voltava os eventos para só MESSAGE (#111). Para desligar um canal
// de propósito, mande o valor explícito ("disabled").
func applyConnectSettings(instance *instance_model.Instance, data *ConnectStruct, warn func(format string, args ...interface{})) []string {
	var subscribedEvents []string

	switch {
	case len(data.Subscribe) == 0:
		if instance.Events != "" {
			subscribedEvents = strings.Split(instance.Events, ",")
		} else {
			subscribedEvents = append(subscribedEvents, event_types.MESSAGE)
		}
	case data.Subscribe[0] == "ALL":
		subscribedEvents = append(subscribedEvents, event_types.AllEventTypes...)
	default:
		for _, arg := range data.Subscribe {
			if !event_types.IsEventType(arg) {
				warn("[%s] Message type discarded '%s'", instance.Id, arg)
				continue
			}
			subscribedEvents = append(subscribedEvents, arg)
		}
	}

	instance.Events = strings.Join(subscribedEvents, ",")
	if data.WebhookUrl != "" {
		instance.Webhook = data.WebhookUrl
	}
	if data.RabbitmqEnable != "" {
		instance.RabbitmqEnable = data.RabbitmqEnable
	}
	if data.NatsEnable != "" {
		instance.NatsEnable = data.NatsEnable
	}
	if data.WebSocketEnable != "" {
		instance.WebSocketEnable = data.WebSocketEnable
	}

	return subscribedEvents
}

func (i instances) Connect(data *ConnectStruct, instance *instance_model.Instance) (*instance_model.Instance, string, string, error) {
	i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Processing subscribe events: %v", instance.Id, data.Subscribe)

	subscribedEvents := applyConnectSettings(instance, data, i.loggerWrapper.GetLogger(instance.Id).LogWarn)
	eventString := instance.Events

	err := i.instanceRepository.Update(instance)
	if err != nil {
		i.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error updating instance: %s", instance.Id, err)
		return nil, "", "", err
	}

	// A instância só conta como "rodando" se o cliente existe E está
	// conectado. Antes bastava existir: um cliente morto (depois de um
	// stream:error, por exemplo) fazia o connect virar um no-op (#185).
	isInstanceRunning := false
	if client := i.clientPointer.Get(instance.Id); client != nil && client.IsConnected() {
		err = i.whatsmeowService.UpdateInstanceSettings(instance.Id)
		if err != nil {
			i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Instance not in runtime yet, will be updated when connected", instance.Id)
		} else {
			i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Instance settings updated successfully in runtime", instance.Id)
			isInstanceRunning = true
		}
	}

	// Se a instância não estiver rodando, inicia uma nova
	if !isInstanceRunning {
		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Starting new client instance", instance.Id)

		clientData := &whatsmeow_service.ClientData{
			Instance:      instance,
			Subscriptions: subscribedEvents,
			Phone:         data.Phone,
			IsProxy:       false,
		}

		if instance.Proxy != "" || i.config.ProxyHost != "" {
			var proxyConfig ProxyConfig
			err := json.Unmarshal([]byte(instance.Proxy), &proxyConfig)
			if err != nil {
				i.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error unmarshalling proxy config: %v", instance.Id, err)
				return nil, "", "", err
			}

			if proxyConfig.Host != "" || i.config.ProxyHost != "" {
				clientData.IsProxy = true
			}
		}

		go i.whatsmeowService.StartClient(clientData)
	} else {
		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Instance already running, settings updated without restarting client", instance.Id)
	}

	// logger.LogInfo("Waiting 1 seconds")
	// time.Sleep(1000 * time.Millisecond)

	// if i.clientPointer.Get(instance.Id) != nil {
	// 	if !i.clientPointer.Get(instance.Id).IsConnected() {
	// 		return instance, "", "", fmt.Errorf("failed to connect")
	// 	}
	// } else {
	// 	return instance, "", "", fmt.Errorf("failed to connect")
	// }

	return instance, instance.Jid, eventString, nil
}

func (i instances) Reconnect(instance *instance_model.Instance) error {
	_, err := i.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}

	return i.whatsmeowService.ReconnectClient(instance.Id)
}

func (i instances) Disconnect(instance *instance_model.Instance) (*instance_model.Instance, error) {
	client := i.clientPointer.Get(instance.Id)
	wasLoggedIn := client != nil && client.IsConnected() && client.IsLoggedIn()

	// Para o cliente mesmo que ele esteja travado/desconectado; antes um
	// cliente morto fazia o disconnect falhar com "no active session found"
	// e só reiniciar o processo resolvia (#185). O cliente não é reiniciado.
	if !i.whatsmeowService.StopClient(instance.Id, "disconnected via API") {
		i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Ignoring disconnect as it was not connected", instance.Id)
		return instance, nil
	}

	i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Disconnection successful", instance.Id)

	if wasLoggedIn {
		instance.Events = ""

		err := i.instanceRepository.Update(instance)
		if err != nil {
			return instance, err
		}
	}

	return instance, nil
}

func (i instances) Logout(instance *instance_model.Instance) (*instance_model.Instance, error) {
	client, err := i.ensureClientConnected(instance.Id)
	if err != nil {
		return instance, err
	}

	if client.IsLoggedIn() && client.IsConnected() {
		err := client.Logout(context.Background())
		if err != nil {
			return instance, err
		}

		instance.Connected = false
		err = i.instanceRepository.Update(instance)
		if err != nil {
			return instance, err
		}

		i.whatsmeowService.StopClient(instance.Id, "logged out via API")

		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Logout successful", instance.Id)
		return instance, nil
	}

	if client.IsConnected() {
		i.whatsmeowService.StopClient(instance.Id, "disconnected via API")

		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Disconnection successful", instance.Id)
		return instance, nil
	}

	i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Ignoring logout as it was not connected", instance.Id)
	return instance, fmt.Errorf("ignoring logout as it was not connected")
}

func (i instances) Status(instance *instance_model.Instance) (*StatusStruct, error) {
	client := i.clientPointer.Get(instance.Id)

	if client == nil {
		return &StatusStruct{
			Connected: false,
			LoggedIn:  false,
		}, nil
	}

	isConnected := client.IsConnected()
	isLoggedIn := client.IsLoggedIn()

	var myJid *types.JID
	var name string
	if isLoggedIn {
		myJid = client.Store.ID
		name = client.Store.PushName
	}

	return &StatusStruct{
		Connected: isConnected,
		LoggedIn:  isLoggedIn,
		myJid:     myJid,
		Name:      name,
	}, nil
}

func (i instances) GetQr(instance *instance_model.Instance) (*QrcodeStruct, error) {
	logger := i.loggerWrapper.GetLogger(instance.Id)
	client := i.clientPointer.Get(instance.Id)

	// Se não há cliente ou o cliente está logado, precisamos iniciar um novo cliente
	if client == nil || client.IsLoggedIn() {
		if client != nil && client.IsLoggedIn() {
			logger.LogInfo("[%s] Client is logged in, starting new instance for QR code", instance.Id)
		} else {
			logger.LogInfo("[%s] No client found, starting new instance for QR code", instance.Id)
		}

		// Iniciar nova instância para gerar QR code
		err := i.whatsmeowService.StartInstance(instance.Id)
		if err != nil {
			logger.LogError("[%s] Failed to start instance: %v", instance.Id, err)
			return nil, fmt.Errorf("failed to start instance: %w", err)
		}

		// Aguardar um pouco para o cliente iniciar e gerar QR code
		logger.LogInfo("[%s] Waiting for QR code generation...", instance.Id)
		time.Sleep(3 * time.Second)

		// Verificar novamente se há cliente
		client = i.clientPointer.Get(instance.Id)
		if client != nil && client.IsLoggedIn() {
			return nil, fmt.Errorf("session already logged in")
		}
	} else if !client.IsConnected() {
		// Se o cliente existe mas não está conectado, pode estar aguardando QR code
		logger.LogInfo("[%s] Client exists but not connected, checking for existing QR code", instance.Id)
	}

	// Buscar instância atualizada do banco para pegar o QR code mais recente
	instance, err := i.instanceRepository.GetInstanceByID(instance.Id)
	if err != nil {
		return nil, err
	}

	// If a passkey ceremony is in progress, there is no QR to scan — return the
	// passkey stage + the #wapk openUrl so the manager can render the
	// "Abrir WhatsApp Web" button. Checked before the empty-QR branch because
	// during a passkey ceremony instance.Qrcode is empty.
	if store := i.whatsmeowService.PasskeyCeremonyStore(); store != nil {
		if token, state, ok := store.StateByInstance(instance.Id); ok {
			logger.LogInfo("[%s] Passkey ceremony active (stage=%s) — returning passkey info instead of QR", instance.Id, state.Stage)
			return &QrcodeStruct{
				PasskeyStage:   state.Stage,
				PasskeyCode:    state.Code,
				PasskeyOpenURL: buildPasskeyOpenURL(token),
			}, nil
		}
	}

	code := instance.Qrcode
	if code == "" {
		// Se não há QR code ainda, aguardar um pouco mais e tentar novamente
		logger.LogInfo("[%s] No QR code available yet, waiting a bit more...", instance.Id)
		time.Sleep(2 * time.Second)

		instance, err = i.instanceRepository.GetInstanceByID(instance.Id)
		if err != nil {
			return nil, err
		}

		code = instance.Qrcode
		if code == "" {
			return nil, fmt.Errorf("no QR code available. Please wait a moment and try again")
		}
	}

	parts := strings.Split(code, "|")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid QR code format")
	}

	qr := &QrcodeStruct{
		Qrcode: parts[0],
		Code:   parts[1],
	}

	return qr, nil
}

// buildPasskeyOpenURL builds the URL the manager opens to start the passkey
// ceremony: https://web.whatsapp.com/#wapk=<base64url({t:token,b:publicBase})>.
// publicBase must be the PUBLICLY reachable API base the browser can hit; set it
// via PASSKEY_PUBLIC_URL. Kept in sync with the event handler in whatsmeow.go.
func buildPasskeyOpenURL(token string) string {
	publicBase := os.Getenv("PASSKEY_PUBLIC_URL")
	if publicBase == "" {
		publicBase = "<SET_PASSKEY_PUBLIC_URL>"
	}
	payload := fmt.Sprintf(`{"t":%q,"b":%q}`, token, publicBase)
	wapk := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return "https://web.whatsapp.com/#wapk=" + wapk
}

func (i instances) Pair(data *PairStruct, instance *instance_model.Instance) (*PairReturnStruct, error) {
	logger := i.loggerWrapper.GetLogger(instance.Id)
	client := i.clientPointer.Get(instance.Id)

	if client == nil || !client.IsConnected() {
		if client != nil && client.IsLoggedIn() {
			return nil, ErrAlreadyAuthenticated
		}
		logger.LogInfo("[%s] No active connection, starting instance for phone pairing", instance.Id)
		if err := i.whatsmeowService.StartInstance(instance.Id); err != nil {
			logger.LogError("[%s] Failed to start instance for pairing: %v", instance.Id, err)
			return nil, fmt.Errorf("failed to start instance: %w", err)
		}
		// Wait for the WA websocket connection and initial QR generation to establish.
		// PairPhone must be called after the QR event is received per whatsmeow docs.
		time.Sleep(3 * time.Second)
		client = i.clientPointer.Get(instance.Id)
		if client == nil {
			return nil, fmt.Errorf("failed to initialize client for pairing")
		}
	}

	if client.IsLoggedIn() {
		return nil, ErrAlreadyAuthenticated
	}

	code, err := client.PairPhone(context.Background(), data.Phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		logger.LogError("[%s] PairPhone failed: %v", instance.Id, err)
		return nil, fmt.Errorf("pairing failed: %w", err)
	}

	return &PairReturnStruct{PairingCode: code}, nil
}

func (i instances) GetAll() ([]*instance_model.Instance, error) {
	instances, err := i.instanceRepository.GetAll(i.config.ClientName)
	if err != nil {
		return nil, err
	}

	for _, instance := range instances {
		if client := i.clientPointer.Get(instance.Id); client != nil {
			instance.Connected = client.IsLoggedIn()
		} else {
			instance.Connected = false
		}

		instance.Proxy = ""
	}

	return instances, nil
}

func (i instances) Info(instanceId string) (*instance_model.Instance, error) {
	instance, err := i.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return nil, err
	}

	// Atualiza o status connected com base no estado real do cliente
	if client := i.clientPointer.Get(instance.Id); client != nil {
		instance.Connected = client.IsLoggedIn()
	} else {
		instance.Connected = false
	}

	instance.Proxy = ""

	return instance, nil
}

func (i instances) Delete(id string) error {
	instance, err := i.instanceRepository.GetInstanceByID(id)
	if err != nil {
		return err
	}

	if client := i.clientPointer.Get(instance.Id); client != nil && client.IsConnected() && client.IsLoggedIn() {
		if err := client.Logout(context.Background()); err != nil {
			i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Logout before delete failed: %v", instance.Id, err)
		}
	}

	// Para o cliente e espera a limpeza. Antes o kill channel era fechado, o
	// que fazia o loop do StartClient reiniciar o cliente de uma instância que
	// estava sendo apagada (#222).
	i.whatsmeowService.StopClient(instance.Id, "instance deleted")

	// Limpar cache via whatsmeow service
	err = i.whatsmeowService.ClearInstanceCache(instance.Id, instance.Token)
	if err != nil {
		i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Failed to clear instance cache: %v", instance.Id, err)
	}

	err = i.instanceRepository.Delete(id)
	if err != nil {
		return err
	}

	return nil
}

func (i instances) SetProxy(id string, proxyConfig *ProxyConfig) error {
	instance, err := i.instanceRepository.GetInstanceByID(id)
	if err != nil {
		return err
	}

	// Validate proxy configuration
	if proxyConfig == nil {
		return fmt.Errorf("proxy configuration cannot be nil")
	}

	if proxyConfig.Host == "" {
		return fmt.Errorf("proxy host is required")
	}

	if proxyConfig.Port == "" {
		return fmt.Errorf("proxy port is required")
	}

	proxyConfig.Protocol = utils.NormalizeProxyProtocol(proxyConfig.Protocol, proxyConfig.Port)

	// Convert proxy config to JSON
	proxyJSON, err := json.Marshal(proxyConfig)
	if err != nil {
		i.loggerWrapper.GetLogger(id).LogError("[%s] Failed to marshal proxy config: %v", id, err)
		return fmt.Errorf("failed to marshal proxy configuration: %v", err)
	}

	instance.Proxy = string(proxyJSON)

	// Update instance in database
	err = i.instanceRepository.Update(instance)
	if err != nil {
		i.loggerWrapper.GetLogger(id).LogError("[%s] Failed to update instance with proxy: %v", id, err)
		return err
	}

	i.loggerWrapper.GetLogger(id).LogInfo("[%s] Proxy configuration updated: %s://%s:%s", id, proxyConfig.Protocol, proxyConfig.Host, proxyConfig.Port)

	// Reconnect to apply proxy changes
	go i.Reconnect(instance)

	return nil
}

func (i instances) SetProxyFromStruct(id string, data *SetProxyStruct) error {
	if data == nil {
		return fmt.Errorf("proxy data cannot be nil")
	}

	proxyConfig := &ProxyConfig{
		Protocol: data.Protocol,
		Host:     data.Host,
		Port:     data.Port,
		Username: data.Username,
		Password: data.Password,
	}

	return i.SetProxy(id, proxyConfig)
}

func (i instances) RemoveProxy(id string) error {
	instance, err := i.instanceRepository.GetInstanceByID(id)
	if err != nil {
		return err
	}

	instance.Proxy = ""

	err = i.instanceRepository.Update(instance)
	if err != nil {
		return err
	}

	i.loggerWrapper.GetLogger(id).LogInfo("[%s] Proxy configuration removed", id)

	go i.Reconnect(instance)

	return nil
}

func (i instances) ForceReconnect(instanceId string, number string) error {
	// Antes isto dava nil pointer (HTTP 500) quando não havia cliente (#185).
	if client := i.clientPointer.Get(instanceId); client != nil && client.IsConnected() && client.IsLoggedIn() {
		return fmt.Errorf("client already connected")
	}

	err := i.whatsmeowService.ForceUpdateJid(instanceId, number)
	if err != nil {
		return err
	}

	instance, err := i.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return err
	}

	subscribedEvents := strings.Split(instance.Events, ",")

	clientData := &whatsmeow_service.ClientData{
		Instance:      instance,
		Subscriptions: subscribedEvents,
		Phone:         "",
		IsProxy:       false,
	}

	if instance.Proxy != "" || i.config.ProxyHost != "" {
		var proxyConfig ProxyConfig
		err := json.Unmarshal([]byte(instance.Proxy), &proxyConfig)
		if err != nil {
			i.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error unmarshalling proxy config: %v", instance.Id, err)
			return err
		}

		if proxyConfig.Host != "" || i.config.ProxyHost != "" {
			clientData.IsProxy = true
		}
	}

	// Para o cliente antigo (se houver) e espera a limpeza antes de iniciar o novo.
	i.whatsmeowService.StopClient(instance.Id, "reconnecting")

	go i.whatsmeowService.StartClient(clientData)

	time.Sleep(2 * time.Second)

	client := i.clientPointer.Get(instance.Id)
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("failed to connect")
	}
	if !client.IsLoggedIn() {
		return fmt.Errorf("failed to login")
	}

	return nil
}

func (i instances) GetInstanceByToken(token string) (*instance_model.Instance, error) {
	return i.instanceRepository.GetInstanceByToken(token)
}

func (i instances) GetLogs(instanceId string, startDate, endDate time.Time, level string, limit int) ([]logger_wrapper.LogEntry, error) {
	// Inicializa o slice vazio para garantir que nunca retorne null
	logs := make([]logger_wrapper.LogEntry, 0)

	// Define valores padrão
	if limit <= 0 {
		limit = 100 // Limite padrão de 100 registros
	}

	// Se não foi fornecida data inicial, usa 7 dias atrás
	if startDate.IsZero() {
		startDate = time.Now().AddDate(0, 0, -7)
	}

	// Se não foi fornecida data final, usa data atual
	if endDate.IsZero() {
		endDate = time.Now()
	}

	// Ajusta as datas para início e fim do dia
	startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.UTC)
	endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, time.UTC)

	// Garante que a data inicial não seja posterior à data final
	if startDate.After(endDate) {
		return logs, fmt.Errorf("data inicial não pode ser posterior à data final")
	}

	// Níveis de log válidos
	validLevels := map[string]bool{
		"INFO":  true,
		"ERROR": true,
		"WARN":  true,
		"DEBUG": true,
	}

	var levelArray []string
	if level == "" {
		// Se nenhum nível foi especificado, usa todos
		levelArray = []string{"INFO", "ERROR", "WARN", "DEBUG"}
	} else {
		// Divide e normaliza os níveis fornecidos
		for _, l := range strings.Split(level, ",") {
			l = strings.TrimSpace(strings.ToUpper(l))
			if !validLevels[l] {
				return logs, fmt.Errorf("nível de log inválido: %s", l)
			}
			levelArray = append(levelArray, l)
		}
	}

	// Lê os logs do arquivo
	logPath := filepath.Join(i.config.LogDirectory, instanceId, "instance.log")
	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return logs, nil // Retorna array vazio se arquivo não existir
		}
		return logs, fmt.Errorf("erro ao abrir arquivo de log: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Aumenta o buffer do scanner para lidar com linhas grandes
	const maxCapacity = 1024 * 1024 // 1MB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		var entry logger_wrapper.LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // Ignora linhas inválidas
		}

		// Ajusta o timestamp da entrada para UTC para comparação correta
		entry.Timestamp = entry.Timestamp.UTC()

		// Aplica os filtros
		if entry.Timestamp.Before(startDate) || entry.Timestamp.After(endDate) {
			continue
		}

		if !slices.Contains(levelArray, entry.Level) {
			continue
		}

		logs = append(logs, entry)

		// Verifica o limite
		if len(logs) >= limit {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return logs, fmt.Errorf("erro ao ler arquivo de log: %v", err)
	}

	// Ordena os logs por timestamp em ordem decrescente
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Timestamp.After(logs[j].Timestamp)
	})

	return logs, nil
}

func (i instances) GetAdvancedSettings(instanceId string) (*instance_model.AdvancedSettings, error) {
	i.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Getting advanced settings", instanceId)

	settings, err := i.instanceRepository.GetAdvancedSettings(instanceId)
	if err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error getting advanced settings: %v", instanceId, err)
		return nil, err
	}

	return settings, nil
}

func (i instances) UpdateAdvancedSettings(instanceId string, settings *instance_model.AdvancedSettings) error {
	i.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Updating advanced settings", instanceId)

	err := i.instanceRepository.UpdateAdvancedSettings(instanceId, settings)
	if err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error updating advanced settings: %v", instanceId, err)
		return err
	}

	// Sincroniza as configurações na instância em execução
	err = i.whatsmeowService.UpdateInstanceAdvancedSettings(instanceId)
	if err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Error syncing advanced settings to runtime: %v", instanceId, err)
		// Não falha a operação, apenas loga o warning
	}

	i.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Advanced settings updated successfully", instanceId)
	return nil
}

func NewInstanceService(
	instanceRepository instance_repository.InstanceRepository,
	clientPointer *runtimestate.Map[string, *whatsmeow.Client],
	whatsmeowService whatsmeow_service.WhatsmeowService,
	config *config.Config,
	loggerWrapper *logger_wrapper.LoggerManager,
) InstanceService {
	return &instances{
		instanceRepository: instanceRepository,
		clientPointer:      clientPointer,
		whatsmeowService:   whatsmeowService,
		config:             config,
		loggerWrapper:      loggerWrapper,
	}
}
