package handler

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	l "github.com/labstack/gommon/log"

	"github.com/AuxXxilium/docker-ddns-server/dyndns/nswrapper"

	"github.com/AuxXxilium/docker-ddns-server/dyndns/model"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

const (
	UNAUTHORIZED = "You are not allowed to view that content"
)

// GetHost fetches a host from the database by "id".
func (h *Handler) GetHost(c echo.Context) (err error) {
	if !h.AuthAdmin {
		return c.JSON(http.StatusUnauthorized, &Error{UNAUTHORIZED})
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	host := &model.Host{}
	if err = h.DB.First(host, id).Error; err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	// Display site
	return c.JSON(http.StatusOK, id)
}

// ListHosts fetches all hosts from database and lists them on the website.
func (h *Handler) ListHosts(c echo.Context) (err error) {
	if !h.AuthAdmin {
		return c.JSON(http.StatusUnauthorized, &Error{UNAUTHORIZED})
	}

	hosts := new([]model.Host)
	if err = h.DB.Find(hosts).Error; err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	return c.Render(http.StatusOK, "listhosts", echo.Map{
		"hosts": hosts,
		"title": h.Title,
	})
}

// AddHost just renders the "add host" website.
func (h *Handler) AddHost(c echo.Context) (err error) {
	if !h.AuthAdmin {
		return c.JSON(http.StatusUnauthorized, &Error{UNAUTHORIZED})
	}

	return c.Render(http.StatusOK, "edithost", echo.Map{
		"addEdit": "add",
		"config":  h.Config,
		"title":   h.Title,
	})
}

// EditHost fetches a host by "id" and renders the "edit host" website.
func (h *Handler) EditHost(c echo.Context) (err error) {
	if !h.AuthAdmin {
		return c.JSON(http.StatusUnauthorized, &Error{UNAUTHORIZED})
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	host := &model.Host{}
	if err = h.DB.First(host, id).Error; err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	return c.Render(http.StatusOK, "edithost", echo.Map{
		"host":    host,
		"addEdit": "edit",
		"config":  h.Config,
		"title":   h.Title,
	})
}

// CreateHost validates the host data from the "add host" website,
// adds the host entry to the database,
// and adds the entry to the DNS server.
func (h *Handler) CreateHost(c echo.Context) (err error) {
	if !h.AuthAdmin {
		return c.JSON(http.StatusUnauthorized, &Error{UNAUTHORIZED})
	}

	host := &model.Host{}
	if err = c.Bind(host); err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	if err = c.Validate(host); err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	if err = h.checkUniqueHostname(host.Hostname, host.Domain); err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}
	host.LastUpdate = time.Now()
	if err = h.DB.Create(host).Error; err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	// If a ip is set create dns entry
	if host.Ip != "" {
		ipType := nswrapper.GetIPType(host.Ip)
		if ipType == "" {
			return c.JSON(http.StatusBadRequest, &Error{fmt.Sprintf("ip %s is not a valid ip", host.Ip)})
		}

		if err = nswrapper.UpdateRecord(host.Hostname, host.Ip, ipType, host.Domain, host.Ttl, h.AllowWildcard); err != nil {
			return c.JSON(http.StatusBadRequest, &Error{err.Error()})
		}
	}

	return c.JSON(http.StatusOK, host)
}

// UpdateHost validates the host data from the "edit host" website,
// and compares the host data with the entry in the database by "id".
// If anything has changed the database and DNS entries for the host will be updated.
func (h *Handler) UpdateHost(c echo.Context) (err error) {
	if !h.AuthAdmin {
		return c.JSON(http.StatusUnauthorized, &Error{UNAUTHORIZED})
	}

	hostUpdate := &model.Host{}
	if err = c.Bind(hostUpdate); err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	host := &model.Host{}
	if err = h.DB.First(host, id).Error; err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	forceRecordUpdate := host.UpdateHost(hostUpdate)
	if err = c.Validate(host); err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	if err = h.DB.Save(host).Error; err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	// If ip or ttl changed update dns entry
	if forceRecordUpdate {
		ipType := nswrapper.GetIPType(host.Ip)
		if ipType == "" {
			return c.JSON(http.StatusBadRequest, &Error{fmt.Sprintf("ip %s is not a valid ip", host.Ip)})
		}

		if err = nswrapper.UpdateRecord(host.Hostname, host.Ip, ipType, host.Domain, host.Ttl, h.AllowWildcard); err != nil {
			return c.JSON(http.StatusBadRequest, &Error{err.Error()})
		}
	}

	return c.JSON(http.StatusOK, host)
}

// DeleteHost fetches a host entry from the database by "id"
// and deletes the database and DNS server entry to it.
func (h *Handler) DeleteHost(c echo.Context) (err error) {
	if !h.AuthAdmin {
		return c.JSON(http.StatusUnauthorized, &Error{UNAUTHORIZED})
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	host := &model.Host{}
	if err = h.DB.First(host, id).Error; err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if err = tx.Unscoped().Delete(host).Error; err != nil {
			return c.JSON(http.StatusBadRequest, &Error{err.Error()})
		}

		if err = tx.Where(&model.Log{HostID: uint(id)}).Delete(&model.Log{}).Error; err != nil {
			return c.JSON(http.StatusBadRequest, &Error{err.Error()})
		}

		if err = tx.Where(&model.CName{TargetID: uint(id)}).Delete(&model.CName{}).Error; err != nil {
			return c.JSON(http.StatusBadRequest, &Error{err.Error()})
		}

		return nil
	})
	if err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	if err = nswrapper.DeleteRecord(host.Hostname, host.Domain, h.AllowWildcard); err != nil {
		return c.JSON(http.StatusBadRequest, &Error{err.Error()})
	}

	return c.JSON(http.StatusOK, id)
}

// UpdateIP implements the update method called by the routers.
// Hostname, IP and senders IP are validated, a log entry is created
// and finally if everything is ok, the DNS Server will be updated
func (h *Handler) UpdateIP(c echo.Context) (err error) {
	host, ok := c.Get("updateHost").(*model.Host)
	if !ok {
		return c.String(http.StatusBadRequest, "badauth\n")
	}

	log := &model.Log{Status: false, Host: *host, TimeStamp: time.Now(), UserAgent: nswrapper.ShrinkUserAgent(c.Request().UserAgent())}
	log.SentIP = c.QueryParam(("myip"))

	// Get caller IP
	log.CallerIP, _ = nswrapper.GetCallerIP(c.Request())
	if log.CallerIP == "" {
		log.CallerIP, _, err = net.SplitHostPort(c.Request().RemoteAddr)
		if err != nil {
			log.Message = "Bad Request: Unable to get caller IP"
			if err = h.CreateLogEntry(log); err != nil {
				l.Error(err)
			}

			return c.String(http.StatusBadRequest, "badrequest\n")
		}
	}

	// Validate hostname
	hostname := c.QueryParam("hostname")
	if hostname == "" || hostname != host.Hostname+"."+host.Domain {
		log.Message = "Hostname or combination of authenticated user and hostname is invalid"
		if err = h.CreateLogEntry(log); err != nil {
			l.Error(err)
		}

		return c.String(http.StatusBadRequest, "notfqdn\n")
	}

	// Get IP type
	ipType := nswrapper.GetIPType(log.SentIP)
	if ipType == "" {
		log.SentIP = log.CallerIP
		ipType = nswrapper.GetIPType(log.SentIP)
		if ipType == "" {
			log.Message = "Bad Request: Sent IP is invalid"
			if err = h.CreateLogEntry(log); err != nil {
				l.Error(err)
			}

			return c.String(http.StatusBadRequest, "badrequest\n")
		}
	}

	// Add/update DNS record
	if err = nswrapper.UpdateRecord(log.Host.Hostname, log.SentIP, ipType, log.Host.Domain, log.Host.Ttl, h.AllowWildcard); err != nil {
		log.Message = fmt.Sprintf("DNS error: %v", err)
		l.Error(log.Message)
		if err = h.CreateLogEntry(log); err != nil {
			l.Error(err)
		}
		return c.String(http.StatusBadRequest, "dnserr\n")
	}

	// Update DB host entry
	log.Host.Ip = log.SentIP
	log.Host.LastUpdate = log.TimeStamp

	if err = h.DB.Save(log.Host).Error; err != nil {
		return c.JSON(http.StatusBadRequest, "badrequest\n")
	}

	log.Status = true
	log.Message = "No errors occurred"
	if err = h.CreateLogEntry(log); err != nil {
		l.Error(err)
	}

	return c.String(http.StatusOK, "good\n")
}

func (h *Handler) checkUniqueHostname(hostname, domain string) error {
	hosts := new([]model.Host)
	if err := h.DB.Where(&model.Host{Hostname: hostname, Domain: domain}).Find(hosts).Error; err != nil {
		return err
	}

	if len(*hosts) > 0 {
		return fmt.Errorf("hostname already exists")
	}

	cnames := new([]model.CName)
	if err := h.DB.Preload("Target").Where(&model.CName{Hostname: hostname}).Find(cnames).Error; err != nil {
		return err
	}

	for _, cname := range *cnames {
		if cname.Target.Domain == domain {
			return fmt.Errorf("hostname already exists")
		}
	}

	return nil
}
