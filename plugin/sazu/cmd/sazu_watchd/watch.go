package main

import (
	"fmt"

	"github.com/coredns/coredns/plugin/sazu"
)

// zoneState is this process's own, purely in-memory memory of whether a
// zone's chain-of-trust check last succeeded -- the "last known good"
// SAZU-PLAN.md's design for this daemon compares against. Not persisted:
// a restart just re-establishes a fresh baseline on its first pass rather
// than resuming exactly where a previous run left off. That's a
// deliberate simplification for this first cut, not an oversight -- the
// failure mode is narrow (a zone that broke and recovered entirely within
// one restart window goes unremarked) and safe (nothing is ever
// mis-reported, an alert is just possibly missed once), which is an
// acceptable trade for not needing yet another persisted table for a
// purely advisory monitoring signal.
type zoneState struct {
	lastOK bool
}

// Alert is one delegation-change notification a check pass decided
// should go out: zone's chain-of-trust check transitioned since the
// last pass -- either just started failing (Recovered false, Err set)
// or just started passing again after having failed (Recovered true,
// Err nil).
type Alert struct {
	Zone      string
	Addresses []string
	Recovered bool
	Err       error
}

// checkOnce runs one pass over every zone db knows about, re-validating
// each one's chain of trust via validator exactly the way first contact
// (and a §10.4 key rollover) already does -- "does a DS matching this
// zone's pinned key exist at the parent" -- and returns the alerts (if
// any) that a state transition since the last pass warrants. state is
// mutated in place so the next call sees this pass's results.
//
// A zone's very first observation (not yet in state at all) only
// establishes a baseline; it never alerts on its own, even if the check
// already fails then. That's what "compares against last known good"
// means literally -- there is no "last known" yet to have changed from --
// and avoids an alert storm at daemon startup for every zone that
// happens to already be in a known, unaddressed failure state.
func checkOnce(db *sazu.DB, validator sazu.ChainValidator, state map[string]*zoneState) ([]Alert, error) {
	zones, err := db.ListZones()
	if err != nil {
		return nil, fmt.Errorf("listing zones: %w", err)
	}

	var alerts []Alert
	for _, zone := range zones {
		key, ok, err := db.LoadKey(zone)
		if err != nil {
			return nil, fmt.Errorf("loading key for %s: %w", zone, err)
		}
		if !ok {
			// Shouldn't normally happen (every zones row gets a pinned
			// key at first contact) -- nothing to check without one.
			continue
		}

		checkErr := validator.VerifyChainOfTrust(zone, key)
		nowOK := checkErr == nil

		st, seen := state[zone]
		if !seen {
			st = &zoneState{}
			state[zone] = st
		}

		switch {
		case !seen:
			// First observation: baseline only, no alert -- see doc comment.
		case st.lastOK && !nowOK:
			alerts = append(alerts, Alert{Zone: zone, Addresses: contactAddresses(db, zone), Err: checkErr})
		case !st.lastOK && nowOK:
			alerts = append(alerts, Alert{Zone: zone, Addresses: contactAddresses(db, zone), Recovered: true})
		}
		st.lastOK = nowOK
	}
	return alerts, nil
}

// contactAddresses looks up zone's registered §10.6 contact, tolerating
// a lookup failure (or no contact registered at all) by returning no
// addresses rather than failing the whole check pass over it -- an alert
// with nothing to notify still gets logged by the caller, which is
// itself a useful signal ("this zone broke and nobody would have heard
// about it").
func contactAddresses(db *sazu.DB, zone string) []string {
	addrs, _, _ := db.LoadContact(zone)
	return addrs
}
