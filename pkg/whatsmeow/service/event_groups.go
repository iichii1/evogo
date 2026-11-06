package whatsmeow_service

import (
	"github.com/evolution-foundation/evolution-go/pkg/internal/event_types"
	"go.mau.fi/whatsmeow/types/events"
)

// globalEventGroup devolve o grupo de eventos (os mesmos valores de
// event_types) a que um evento do whatsmeow pertence, ou "" se ele não pertence
// a nenhum. É usado nos filtros das filas globais (AMQP e NATS); antes cada uma
// mantinha a própria cópia do mapeamento e elas divergiam: o NATS não publicava
// PICTURE, USER_ABOUT nem BUTTON_CLICK (#193) e nenhuma delas conhecia os
// eventos de passkey (#105).
func globalEventGroup(eventType string) string {
	switch eventType {
	case "Message", "UndecryptableMessage":
		return event_types.MESSAGE
	case "SendMessage":
		return event_types.SEND_MESSAGE
	case "Receipt":
		return event_types.READ_RECEIPT
	case "Presence":
		return event_types.PRESENCE
	case "HistorySync":
		return event_types.HISTORY_SYNC
	case "ChatPresence", "Archive":
		return event_types.CHAT_PRESENCE
	case "MarkChatAsRead", "DeleteChat", "ClearChat", "DeleteForMe", "Mute", "Pin", "Star",
		"UserStatusMute", "Blocklist", "PrivacySettings", "UnarchiveChatsSetting":
		return event_types.CHAT_SYNC
	case "CallOffer", "CallAccept", "CallTerminate", "CallOfferNotice", "CallRelayLatency",
		"CallReject", "CallPreAccept", "CallTransport":
		return event_types.CALL
	case "Connected", "PairSuccess", "TemporaryBan", "AccountReachoutTimelock", "LoggedOut", "ConnectFailure", "Disconnected":
		return event_types.CONNECTION
	case "LabelEdit", "LabelAssociationChat", "LabelAssociationMessage":
		return event_types.LABEL
	case "Contact", "PushName", "BusinessName":
		return event_types.CONTACT
	case "GroupInfo", "JoinedGroup":
		return event_types.GROUP
	case "NewsletterJoin", "NewsletterLeave":
		return event_types.NEWSLETTER
	case "ClientOutdated", "StreamError", "KeepAliveTimeout", "KeepAliveRestored", "AppStateSyncError",
		"CATRefreshError", "OfflineSyncPreview", "OfflineSyncCompleted":
		return event_types.CONNECTION
	case "QRCode", "QRTimeout", "QRSuccess":
		return event_types.QRCODE
	case "ButtonClick":
		return event_types.BUTTON_CLICK
	case "Picture":
		return event_types.PICTURE
	case "UserAbout":
		return event_types.USER_ABOUT
	case "PasskeyRequest", "PasskeyConfirmation", "PasskeyError":
		return event_types.PASSKEY
	default:
		return ""
	}
}

// forwardedEvent diz se um evento do whatsmeow que o handler não trata de forma
// especial deve seguir para o webhook, com qual nome e com qual conteúdo. Antes
// estes eventos caíam no default do handler e eram descartados: ler uma conversa
// no celular, por exemplo, nunca chegava a quem consome o webhook.
//
// StreamError, AppStateSyncError e CATRefreshError não vão como vieram: o campo
// Raw traz o nó bruto do protocolo, e um `error` vira `{}` em JSON.
func forwardedEvent(rawEvt interface{}) (string, interface{}) {
	switch evt := rawEvt.(type) {
	case *events.MarkChatAsRead:
		return "MarkChatAsRead", evt
	case *events.DeleteChat:
		return "DeleteChat", evt
	case *events.ClearChat:
		return "ClearChat", evt
	case *events.DeleteForMe:
		return "DeleteForMe", evt
	case *events.Mute:
		return "Mute", evt
	case *events.Pin:
		return "Pin", evt
	case *events.Star:
		return "Star", evt
	case *events.UserStatusMute:
		return "UserStatusMute", evt
	case *events.Blocklist:
		return "Blocklist", evt
	case *events.PrivacySettings:
		return "PrivacySettings", evt
	case *events.UnarchiveChatsSetting:
		return "UnarchiveChatsSetting", evt
	case *events.CallReject:
		return "CallReject", evt
	case *events.CallPreAccept:
		return "CallPreAccept", evt
	case *events.CallTransport:
		return "CallTransport", evt
	case *events.BusinessName:
		return "BusinessName", evt
	case *events.ClientOutdated:
		return "ClientOutdated", evt
	case *events.KeepAliveTimeout:
		return "KeepAliveTimeout", evt
	case *events.KeepAliveRestored:
		return "KeepAliveRestored", evt
	case *events.OfflineSyncPreview:
		return "OfflineSyncPreview", evt
	case *events.StreamError:
		return "StreamError", map[string]interface{}{"code": evt.Code}
	case *events.AppStateSyncError:
		return "AppStateSyncError", map[string]interface{}{
			"name": string(evt.Name), "fullSync": evt.FullSync, "error": errorText(evt.Error),
		}
	case *events.CATRefreshError:
		return "CATRefreshError", map[string]interface{}{"error": errorText(evt.Error)}
	}
	return "", nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
