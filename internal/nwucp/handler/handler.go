package handler

import (
	"fmt"
	"net"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"

	"github.com/free5gc/n3iwue/internal/gre"
	"github.com/free5gc/n3iwue/internal/logger"
	"github.com/free5gc/n3iwue/internal/packet/nasPacket"
	"github.com/free5gc/n3iwue/internal/rt_table"
	context "github.com/free5gc/n3iwue/pkg/context"
	"github.com/free5gc/nas"
	"github.com/free5gc/nas/nasType"
)

var naslog *logrus.Entry

func init() {
	naslog = logger.NASLog
}

func HandleRegistrationAccept(n3ueSelf *context.N3UE, nasMsg *nas.Message) {
	naslog.Tracef("Get Registration Accept")

	n3ueSelf.RanUeContext.DLCount.AddOne()
	n3ueSelf.GUTI = nasMsg.GmmMessage.RegistrationAccept.GUTI5G

	// send NAS Registration Complete Msg
	pdu := nasPacket.GetRegistrationComplete(nil)
	SendNasMsg(n3ueSelf.RanUeContext, n3ueSelf.N3IWFRanUe.TCPConnection, pdu)

	time.Sleep(500 * time.Millisecond)
	n3ueSelf.CurrentState <- uint8(context.PduSessionEst)
}

func HandleDLNASTransport(n3ueSelf *context.N3UE, nasMsg *nas.Message) {
	payloadContainer := nasMsg.GmmMessage.DLNASTransport.PayloadContainer
	byteArray := payloadContainer.Buffer[:payloadContainer.Len]
	if err := nasMsg.GsmMessageDecode(&byteArray); err != nil {
		naslog.Errorf("NAS Decode Fail: %+v", err)
		return
	}

	switch nasMsg.GsmMessage.GetMessageType() {
	case nas.MsgTypePDUSessionEstablishmentAccept:
		naslog.Tracef("Get PDUSession Establishment Accept")

		pduAddress, err := nasPacket.GetPDUAddress(nasMsg.GsmMessage.PDUSessionEstablishmentAccept)
		if err != nil {
			naslog.Errorf("GetPDUAddress Fail: %+v", err)
			return
		}

		naslog.Infof("PDU Address: %s", pduAddress.String())
		n3ueSelf.N3ueInfo.DnIPAddr = pduAddress.String()

		newGREName := fmt.Sprintf("%s-id-%d", n3ueSelf.N3ueInfo.GreIfaceName, n3ueSelf.N3ueInfo.XfrmiId)

		custom_rt_table_entry := rt_table.RTTablesEntry{
			ID:   200,
			Name: "n3iwue_gre_route",
		}

		rt_table.CreateRTTablesEntry(&custom_rt_table_entry)

		gre_rule := netlink.NewRule()
		gre_rule.Src = netlink.NewIPNet(pduAddress)
		gre_rule.Table = int(custom_rt_table_entry.ID)

		err = netlink.RuleAdd(gre_rule)
		if err != nil {
			naslog.Errorf("Failed to add rule: %v", err)
			return
		} else {
			naslog.Infof("Successfully added IP: %s to table ID: %s", pduAddress.String(), gre_rule.String())
		}

		var linkGREs map[uint8]*netlink.Link
		if linkGREs, err = gre.SetupGreTunnels(newGREName, n3ueSelf.TemporaryXfrmiName, n3ueSelf.UEInnerAddr.IP,
			n3ueSelf.TemporaryUPIPAddr, pduAddress, n3ueSelf.TemporaryQosInfo); err != nil {
			naslog.Errorf("Setup GRE tunnel %s Fail: %+v", newGREName, err)
			return
		}

		qfiToTargetMap, err := nasPacket.GetQFItoTargetMap(nasMsg.PDUSessionEstablishmentAccept)
		if err != nil {
			naslog.Errorf("GetQFItoTargetMap Fail: %+v", err)
			return
		}

		// Add route
		for qfi, link := range linkGREs {
			tunnel := *link
			priority := 1 // lower is higher (1 ~ 7)

			var remoteAddress nasType.PacketFilterIPv4RemoteAddress
			var ok bool
			if qfi == uint8(1) { // default qfi
				remoteAddress.Address = net.IPv4zero
				remoteAddress.Mask = net.IPv4Mask(0, 0, 0, 0)
				priority = 7
			} else if remoteAddress, ok = qfiToTargetMap[qfi]; !ok {
				naslog.Errorf("not found target address for QFI [%v] from NAS", qfi)
				continue
			}

			naslog.Infof("Add route: QFI[%+v] remote address[%+v]", qfi, remoteAddress)
			upRoute := &netlink.Route{
				LinkIndex: tunnel.Attrs().Index,
				Dst: &net.IPNet{
					IP:   remoteAddress.Address,
					Mask: remoteAddress.Mask,
				},
				Priority: priority,
				Table:    int(custom_rt_table_entry.ID),
			}
			if err := netlink.RouteAdd(upRoute); err != nil {
				naslog.Warnf("netlink.RouteAdd: %+v", err)
			}
		}

		n3ueSelf.PduSessionCount++
		n3ueSelf.CurrentState <- uint8(context.PduSessionCreated)
	}
}
