package legquotstatgrp

import (
	"github.com/quickfixgo/quickfix/fix50sp2/instrumentleg"
	"github.com/quickfixgo/quickfix/fix50sp2/legstipulations"
	"github.com/quickfixgo/quickfix/fix50sp2/nestedparties"
)

func New() *LegQuotStatGrp {
	var m LegQuotStatGrp
	return &m
}

//NoLegs is a repeating group in LegQuotStatGrp
type NoLegs struct {
	//InstrumentLeg is a non-required component for NoLegs.
	InstrumentLeg *instrumentleg.InstrumentLeg
	//LegQty is a non-required field for NoLegs.
	LegQty *float64 `fix:"687"`
	//LegSwapType is a non-required field for NoLegs.
	LegSwapType *int `fix:"690"`
	//LegSettlType is a non-required field for NoLegs.
	LegSettlType *string `fix:"587"`
	//LegSettlDate is a non-required field for NoLegs.
	LegSettlDate *string `fix:"588"`
	//LegStipulations is a non-required component for NoLegs.
	LegStipulations *legstipulations.LegStipulations
	//NestedParties is a non-required component for NoLegs.
	NestedParties *nestedparties.NestedParties
	//LegOrderQty is a non-required field for NoLegs.
	LegOrderQty *float64 `fix:"685"`
}

//LegQuotStatGrp is a fix50sp2 Component
type LegQuotStatGrp struct {
	//NoLegs is a non-required field for LegQuotStatGrp.
	NoLegs []NoLegs `fix:"555,omitempty"`
}

func (m *LegQuotStatGrp) SetNoLegs(v []NoLegs) { m.NoLegs = v }