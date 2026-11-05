package whatsmeow_service

import "github.com/evolution-foundation/evolution-go/pkg/internal/event_types"

// globalEventGroup devolve o grupo de eventos (os mesmos valores de
// event_types) a que um evento do whatsmeow pertence, ou "" se ele não pertence
// a nenhum. É usado nos filtros das filas globais (AMQP e NATS); antes cada uma
// mantinha a própria cópia do mapeamento e elas divergiam: o NATS não publicava
// PICTURE, USER_ABOUT nem BUTTON_CLICK (#193) e nenhuma delas conhecia os
// eventos de passkey (#105).
func globalEventGroup(eventType string) string {
	switch eventType {
	case "Message":
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
	case "CallOffer", "CallAccept", "CallTerminate", "CallOfferNotice", "CallRelayLatency":
		return event_types.CALL
	case "Connected", "PairSuccess", "TemporaryBan", "AccountReachoutTimelock", "LoggedOut", "ConnectFailure", "Disconnected":
		return event_types.CONNECTION
	case "LabelEdit", "LabelAssociationChat", "LabelAssociationMessage":
		return event_types.LABEL
	case "Contact", "PushName":
		return event_types.CONTACT
	case "GroupInfo", "JoinedGroup":
		return event_types.GROUP
	case "NewsletterJoin", "NewsletterLeave":
		return event_types.NEWSLETTER
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
