// Code generated by ent, DO NOT EDIT.

package ent

import (
	"time"

	"github.com/coredns/coredns/plugin/atlas/ent/dnsrr"
	"github.com/coredns/coredns/plugin/atlas/ent/dnszone"
	"github.com/coredns/coredns/plugin/atlas/ent/schema"
	"github.com/rs/xid"
)

// The init function reads all schema descriptors with runtime code
// (default values, validators, hooks and policies) and stitches it
// to their package variables.
func init() {
	dnsrrMixin := schema.DnsRR{}.Mixin()
	dnsrrMixinFields0 := dnsrrMixin[0].Fields()
	_ = dnsrrMixinFields0
	dnsrrFields := schema.DnsRR{}.Fields()
	_ = dnsrrFields
	// dnsrrDescCreatedAt is the schema descriptor for created_at field.
	dnsrrDescCreatedAt := dnsrrMixinFields0[1].Descriptor()
	// dnsrr.DefaultCreatedAt holds the default value on creation for the created_at field.
	dnsrr.DefaultCreatedAt = dnsrrDescCreatedAt.Default.(func() time.Time)
	// dnsrrDescUpdatedAt is the schema descriptor for updated_at field.
	dnsrrDescUpdatedAt := dnsrrMixinFields0[2].Descriptor()
	// dnsrr.DefaultUpdatedAt holds the default value on creation for the updated_at field.
	dnsrr.DefaultUpdatedAt = dnsrrDescUpdatedAt.Default.(func() time.Time)
	// dnsrr.UpdateDefaultUpdatedAt holds the default value on update for the updated_at field.
	dnsrr.UpdateDefaultUpdatedAt = dnsrrDescUpdatedAt.UpdateDefault.(func() time.Time)
	// dnsrrDescName is the schema descriptor for name field.
	dnsrrDescName := dnsrrFields[0].Descriptor()
	// dnsrr.NameValidator is a validator for the "name" field. It is called by the builders before save.
	dnsrr.NameValidator = func() func(string) error {
		validators := dnsrrDescName.Validators
		fns := [...]func(string) error{
			validators[0].(func(string) error),
			validators[1].(func(string) error),
			validators[2].(func(string) error),
		}
		return func(name string) error {
			for _, fn := range fns {
				if err := fn(name); err != nil {
					return err
				}
			}
			return nil
		}
	}()
	// dnsrrDescClass is the schema descriptor for class field.
	dnsrrDescClass := dnsrrFields[3].Descriptor()
	// dnsrr.DefaultClass holds the default value on creation for the class field.
	dnsrr.DefaultClass = dnsrrDescClass.Default.(uint16)
	// dnsrrDescTTL is the schema descriptor for ttl field.
	dnsrrDescTTL := dnsrrFields[4].Descriptor()
	// dnsrr.DefaultTTL holds the default value on creation for the ttl field.
	dnsrr.DefaultTTL = dnsrrDescTTL.Default.(uint32)
	// dnsrr.TTLValidator is a validator for the "ttl" field. It is called by the builders before save.
	dnsrr.TTLValidator = func() func(uint32) error {
		validators := dnsrrDescTTL.Validators
		fns := [...]func(uint32) error{
			validators[0].(func(uint32) error),
			validators[1].(func(uint32) error),
		}
		return func(ttl uint32) error {
			for _, fn := range fns {
				if err := fn(ttl); err != nil {
					return err
				}
			}
			return nil
		}
	}()
	// dnsrrDescActivated is the schema descriptor for activated field.
	dnsrrDescActivated := dnsrrFields[5].Descriptor()
	// dnsrr.DefaultActivated holds the default value on creation for the activated field.
	dnsrr.DefaultActivated = dnsrrDescActivated.Default.(bool)
	// dnsrrDescID is the schema descriptor for id field.
	dnsrrDescID := dnsrrMixinFields0[0].Descriptor()
	// dnsrr.DefaultID holds the default value on creation for the id field.
	dnsrr.DefaultID = dnsrrDescID.Default.(func() xid.ID)
	dnszoneMixin := schema.DnsZone{}.Mixin()
	dnszoneMixinFields0 := dnszoneMixin[0].Fields()
	_ = dnszoneMixinFields0
	dnszoneFields := schema.DnsZone{}.Fields()
	_ = dnszoneFields
	// dnszoneDescCreatedAt is the schema descriptor for created_at field.
	dnszoneDescCreatedAt := dnszoneMixinFields0[1].Descriptor()
	// dnszone.DefaultCreatedAt holds the default value on creation for the created_at field.
	dnszone.DefaultCreatedAt = dnszoneDescCreatedAt.Default.(func() time.Time)
	// dnszoneDescUpdatedAt is the schema descriptor for updated_at field.
	dnszoneDescUpdatedAt := dnszoneMixinFields0[2].Descriptor()
	// dnszone.DefaultUpdatedAt holds the default value on creation for the updated_at field.
	dnszone.DefaultUpdatedAt = dnszoneDescUpdatedAt.Default.(func() time.Time)
	// dnszone.UpdateDefaultUpdatedAt holds the default value on update for the updated_at field.
	dnszone.UpdateDefaultUpdatedAt = dnszoneDescUpdatedAt.UpdateDefault.(func() time.Time)
	// dnszoneDescName is the schema descriptor for name field.
	dnszoneDescName := dnszoneFields[0].Descriptor()
	// dnszone.NameValidator is a validator for the "name" field. It is called by the builders before save.
	dnszone.NameValidator = func() func(string) error {
		validators := dnszoneDescName.Validators
		fns := [...]func(string) error{
			validators[0].(func(string) error),
			validators[1].(func(string) error),
			validators[2].(func(string) error),
		}
		return func(name string) error {
			for _, fn := range fns {
				if err := fn(name); err != nil {
					return err
				}
			}
			return nil
		}
	}()
	// dnszoneDescRrtype is the schema descriptor for rrtype field.
	dnszoneDescRrtype := dnszoneFields[1].Descriptor()
	// dnszone.DefaultRrtype holds the default value on creation for the rrtype field.
	dnszone.DefaultRrtype = dnszoneDescRrtype.Default.(uint16)
	// dnszoneDescClass is the schema descriptor for class field.
	dnszoneDescClass := dnszoneFields[2].Descriptor()
	// dnszone.DefaultClass holds the default value on creation for the class field.
	dnszone.DefaultClass = dnszoneDescClass.Default.(uint16)
	// dnszoneDescTTL is the schema descriptor for ttl field.
	dnszoneDescTTL := dnszoneFields[3].Descriptor()
	// dnszone.DefaultTTL holds the default value on creation for the ttl field.
	dnszone.DefaultTTL = dnszoneDescTTL.Default.(uint32)
	// dnszone.TTLValidator is a validator for the "ttl" field. It is called by the builders before save.
	dnszone.TTLValidator = func() func(uint32) error {
		validators := dnszoneDescTTL.Validators
		fns := [...]func(uint32) error{
			validators[0].(func(uint32) error),
			validators[1].(func(uint32) error),
		}
		return func(ttl uint32) error {
			for _, fn := range fns {
				if err := fn(ttl); err != nil {
					return err
				}
			}
			return nil
		}
	}()
	// dnszoneDescNs is the schema descriptor for ns field.
	dnszoneDescNs := dnszoneFields[4].Descriptor()
	// dnszone.NsValidator is a validator for the "ns" field. It is called by the builders before save.
	dnszone.NsValidator = func() func(string) error {
		validators := dnszoneDescNs.Validators
		fns := [...]func(string) error{
			validators[0].(func(string) error),
			validators[1].(func(string) error),
			validators[2].(func(string) error),
		}
		return func(ns string) error {
			for _, fn := range fns {
				if err := fn(ns); err != nil {
					return err
				}
			}
			return nil
		}
	}()
	// dnszoneDescMbox is the schema descriptor for mbox field.
	dnszoneDescMbox := dnszoneFields[5].Descriptor()
	// dnszone.MboxValidator is a validator for the "mbox" field. It is called by the builders before save.
	dnszone.MboxValidator = func() func(string) error {
		validators := dnszoneDescMbox.Validators
		fns := [...]func(string) error{
			validators[0].(func(string) error),
			validators[1].(func(string) error),
			validators[2].(func(string) error),
		}
		return func(mbox string) error {
			for _, fn := range fns {
				if err := fn(mbox); err != nil {
					return err
				}
			}
			return nil
		}
	}()
	// dnszoneDescRefresh is the schema descriptor for refresh field.
	dnszoneDescRefresh := dnszoneFields[7].Descriptor()
	// dnszone.DefaultRefresh holds the default value on creation for the refresh field.
	dnszone.DefaultRefresh = dnszoneDescRefresh.Default.(uint32)
	// dnszone.RefreshValidator is a validator for the "refresh" field. It is called by the builders before save.
	dnszone.RefreshValidator = func() func(uint32) error {
		validators := dnszoneDescRefresh.Validators
		fns := [...]func(uint32) error{
			validators[0].(func(uint32) error),
			validators[1].(func(uint32) error),
		}
		return func(refresh uint32) error {
			for _, fn := range fns {
				if err := fn(refresh); err != nil {
					return err
				}
			}
			return nil
		}
	}()
	// dnszoneDescRetry is the schema descriptor for retry field.
	dnszoneDescRetry := dnszoneFields[8].Descriptor()
	// dnszone.DefaultRetry holds the default value on creation for the retry field.
	dnszone.DefaultRetry = dnszoneDescRetry.Default.(uint32)
	// dnszone.RetryValidator is a validator for the "retry" field. It is called by the builders before save.
	dnszone.RetryValidator = func() func(uint32) error {
		validators := dnszoneDescRetry.Validators
		fns := [...]func(uint32) error{
			validators[0].(func(uint32) error),
			validators[1].(func(uint32) error),
		}
		return func(retry uint32) error {
			for _, fn := range fns {
				if err := fn(retry); err != nil {
					return err
				}
			}
			return nil
		}
	}()
	// dnszoneDescExpire is the schema descriptor for expire field.
	dnszoneDescExpire := dnszoneFields[9].Descriptor()
	// dnszone.DefaultExpire holds the default value on creation for the expire field.
	dnszone.DefaultExpire = dnszoneDescExpire.Default.(uint32)
	// dnszone.ExpireValidator is a validator for the "expire" field. It is called by the builders before save.
	dnszone.ExpireValidator = func() func(uint32) error {
		validators := dnszoneDescExpire.Validators
		fns := [...]func(uint32) error{
			validators[0].(func(uint32) error),
			validators[1].(func(uint32) error),
		}
		return func(expire uint32) error {
			for _, fn := range fns {
				if err := fn(expire); err != nil {
					return err
				}
			}
			return nil
		}
	}()
	// dnszoneDescMinttl is the schema descriptor for minttl field.
	dnszoneDescMinttl := dnszoneFields[10].Descriptor()
	// dnszone.DefaultMinttl holds the default value on creation for the minttl field.
	dnszone.DefaultMinttl = dnszoneDescMinttl.Default.(uint32)
	// dnszone.MinttlValidator is a validator for the "minttl" field. It is called by the builders before save.
	dnszone.MinttlValidator = func() func(uint32) error {
		validators := dnszoneDescMinttl.Validators
		fns := [...]func(uint32) error{
			validators[0].(func(uint32) error),
			validators[1].(func(uint32) error),
		}
		return func(minttl uint32) error {
			for _, fn := range fns {
				if err := fn(minttl); err != nil {
					return err
				}
			}
			return nil
		}
	}()
	// dnszoneDescActivated is the schema descriptor for activated field.
	dnszoneDescActivated := dnszoneFields[11].Descriptor()
	// dnszone.DefaultActivated holds the default value on creation for the activated field.
	dnszone.DefaultActivated = dnszoneDescActivated.Default.(bool)
	// dnszoneDescID is the schema descriptor for id field.
	dnszoneDescID := dnszoneMixinFields0[0].Descriptor()
	// dnszone.DefaultID holds the default value on creation for the id field.
	dnszone.DefaultID = dnszoneDescID.Default.(func() xid.ID)
}
