package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/approval/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

var (
	t0       = time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC) // Senin 09.00 WIB
	divLog   = uuid.MustParse("019a0000-0000-7000-8000-000000000001")
	divSales = uuid.MustParse("019a0000-0000-7000-8000-000000000002")
)

func amount(s string) *money.Amount {
	a := money.MustParse(s)
	return &a
}

func poPolicies() []domain.Policy {
	return []domain.Policy{
		{ID: uuid.New(), SubjectType: domain.SubjectPurchaseOrder, Level: 1, ApproverRole: "head_finance", ValidFrom: t0.Add(-time.Hour)},
		{ID: uuid.New(), SubjectType: domain.SubjectPurchaseOrder, Level: 2, MinAmount: amount("10000000"), ApproverRole: "director", ValidFrom: t0.Add(-time.Hour)},
		{ID: uuid.New(), SubjectType: domain.SubjectPurchaseOrder, Level: 3, MinAmount: amount("50000000"), ApproverRole: "president_director", ValidFrom: t0.Add(-time.Hour)},
	}
}

func subject(t domain.SubjectType, amt string) domain.Subject {
	return domain.Subject{Type: t, ID: uuid.New(), Label: "PO/2026/10/00001", Link: "/procurement/purchase-orders/1", Amount: money.MustParse(amt)}
}

func levels(ps []domain.Policy) []int {
	out := make([]int, len(ps))
	for i, p := range ps {
		out[i] = p.Level
	}
	return out
}

func TestSelectPoliciesByAmount(t *testing.T) {
	tests := []struct {
		amount string
		want   []int
	}{
		{"10000000", []int{1}},
		{"10000001", []int{1, 2}},
		{"50000000", []int{1, 2}},
		{"75000000", []int{1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.amount, func(t *testing.T) {
			got := domain.SelectPolicies(poPolicies(), subject(domain.SubjectPurchaseOrder, tt.amount), t0)
			assert.Equal(t, tt.want, levels(got))
		})
	}
}

func TestSelectPoliciesHonoursValidityConditionAndMaxAmount(t *testing.T) {
	ended := t0.Add(-time.Minute)
	ps := []domain.Policy{
		{SubjectType: domain.SubjectReturnDisposition, Level: 1, ApproverRole: "head_logistics", ValidFrom: t0.Add(-time.Hour)},
		{SubjectType: domain.SubjectReturnDisposition, Level: 2, Condition: domain.ConditionHasDisposal, ApproverRole: "head_finance", ValidFrom: t0.Add(-time.Hour)},
		{SubjectType: domain.SubjectReturnDisposition, Level: 3, MinAmount: amount("10000000"), Condition: domain.ConditionHasDisposal, ApproverRole: "director", ValidFrom: t0.Add(-time.Hour)},
		// Versi lama level 1 yang sudah ditutup dan kebijakan yang baru berlaku besok diabaikan.
		{SubjectType: domain.SubjectReturnDisposition, Level: 4, ApproverRole: "director", ValidFrom: t0.Add(-2 * time.Hour), ValidTo: &ended},
		{SubjectType: domain.SubjectReturnDisposition, Level: 5, ApproverRole: "director", ValidFrom: t0.Add(24 * time.Hour)},
		{SubjectType: domain.SubjectPurchaseOrder, Level: 6, ApproverRole: "director", ValidFrom: t0.Add(-time.Hour)},
		{SubjectType: domain.SubjectReturnDisposition, Level: 7, MaxAmount: amount("1000"), ApproverRole: "director", ValidFrom: t0.Add(-time.Hour)},
	}
	s := subject(domain.SubjectReturnDisposition, "0")
	assert.Equal(t, []int{1, 7}, levels(domain.SelectPolicies(ps, s, t0)), "tanpa pemusnahan hanya Head Logistik")

	s.Flags = []string{domain.ConditionHasDisposal}
	s.Amount = money.MustParse("12000000")
	assert.Equal(t, []int{1, 2, 3}, levels(domain.SelectPolicies(ps, s, t0)))
}

func TestSelectPoliciesDivisionOverridesGeneral(t *testing.T) {
	special := uuid.New()
	ps := []domain.Policy{
		{ID: special, SubjectType: domain.SubjectExpenseClaim, Division: &divSales, Level: 1, ApproverRole: "head_sales", ValidFrom: t0.Add(-time.Hour)},
		{SubjectType: domain.SubjectExpenseClaim, Level: 1, ApproverRole: "division_head", ValidFrom: t0.Add(-time.Hour)},
		{SubjectType: domain.SubjectExpenseClaim, Division: &divLog, Level: 2, ApproverRole: "head_logistics", ValidFrom: t0.Add(-time.Hour)},
	}
	s := subject(domain.SubjectExpenseClaim, "100")
	s.Division = &divSales
	got := domain.SelectPolicies(ps, s, t0)
	require.Len(t, got, 1)
	assert.Equal(t, special, got[0].ID)

	s.Division = nil
	got = domain.SelectPolicies(ps, s, t0)
	require.Len(t, got, 1)
	assert.Equal(t, "division_head", got[0].ApproverRole)
}

func TestPolicyInputValidate(t *testing.T) {
	user := uuid.New()
	valid := domain.PolicyInput{SubjectType: domain.SubjectPurchaseOrder, Level: 1, ApproverRole: "head_finance"}
	require.NoError(t, valid.Validate())
	p, err := domain.NewPolicy(valid, t0)
	require.NoError(t, err)
	assert.True(t, p.ActiveAt(t0))
	assert.False(t, p.ActiveAt(t0.Add(-time.Second)))

	tests := []struct {
		name  string
		edit  func(*domain.PolicyInput)
		field string
	}{
		{"jenis tidak dikenal", func(in *domain.PolicyInput) { in.SubjectType = "unknown" }, "subjectType"},
		{"level 0", func(in *domain.PolicyInput) { in.Level = 0 }, "level"},
		{"level 10", func(in *domain.PolicyInput) { in.Level = 10 }, "level"},
		{"batas bawah negatif", func(in *domain.PolicyInput) { in.MinAmount = amount("-1") }, "minAmount"},
		{"batas atas nol", func(in *domain.PolicyInput) { in.MaxAmount = amount("0") }, "maxAmount"},
		{"batas atas <= bawah", func(in *domain.PolicyInput) { in.MinAmount, in.MaxAmount = amount("10"), amount("10") }, "maxAmount"},
		{"syarat tidak berlaku", func(in *domain.PolicyInput) { in.Condition = domain.ConditionHasDisposal }, "condition"},
		{"tanpa jabatan", func(in *domain.PolicyInput) { in.ApproverRole = "" }, "approverRole"},
		{"pengganti = approver", func(in *domain.PolicyInput) { in.ApproverUser, in.Substitute = &user, &user }, "substituteUserId"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := valid
			tt.edit(&in)
			_, err := domain.NewPolicy(in, t0)
			require.Error(t, err)
			assert.Contains(t, apperr.As(err).Fields, tt.field)
		})
	}
}

func TestPolicyEnd(t *testing.T) {
	p := domain.Policy{ValidFrom: t0}
	require.Error(t, p.End(t0), "tidak bisa ditutup sebelum mulai berlaku")
	require.NoError(t, p.End(t0.Add(time.Hour)))
	assert.False(t, p.ActiveAt(t0.Add(time.Hour)))
	err := p.End(t0.Add(2 * time.Hour))
	assert.Equal(t, apperr.CodeInvalidStateTransition, apperr.As(err).Code)
}

func principal(roles ...string) domain.Principal {
	p := domain.Principal{UserID: uuid.New(), Name: "User " + roles[0]}
	for _, r := range roles {
		p.Roles = append(p.Roles, domain.RoleGrant{Code: r})
	}
	return p
}

func newPORequest(t *testing.T, requester domain.Principal, amt string) domain.Request {
	t.Helper()
	s := subject(domain.SubjectPurchaseOrder, amt)
	r, err := domain.NewRequest(s, requester, domain.SelectPolicies(poPolicies(), s, t0), t0)
	require.NoError(t, err)
	return r
}

func TestNewRequestValidation(t *testing.T) {
	purchasing := principal("purchasing")
	_, err := domain.NewRequest(subject(domain.SubjectPurchaseOrder, "1"), purchasing, nil, t0)
	assert.Equal(t, apperr.CodeApprovalPolicyMissing, apperr.As(err).Code)

	bad := domain.Subject{Type: "x", Amount: money.MustParse("-1"), Link: "http://evil"}
	_, err = domain.NewRequest(bad, purchasing, poPolicies(), t0)
	fields := apperr.As(err).Fields
	for _, f := range []string{"subjectType", "subjectId", "subjectLabel", "amount", "subjectLink"} {
		assert.Contains(t, fields, f)
	}
}

func TestApproveAllLevelsInOrder(t *testing.T) {
	purchasing := principal("purchasing")
	hfa, dir, pd := principal("head_finance"), principal("director"), principal("president_director")
	r := newPORequest(t, purchasing, "60000000")
	require.Equal(t, 3, r.TotalLevels())
	require.NotNil(t, r.Steps[0].ActivatedAt)
	assert.Nil(t, r.Steps[1].ActivatedAt, "level berikutnya belum aktif")

	_, err := r.Authorize(dir, nil)
	assert.Equal(t, apperr.CodeForbidden, apperr.As(err).Code, "Direksi belum boleh memutuskan level 1")

	for i, p := range []domain.Principal{hfa, dir, pd} {
		from, err := r.Authorize(p, nil)
		require.NoError(t, err)
		assert.Nil(t, from)
		done, err := r.Approve(p, nil, "ok", t0.Add(time.Duration(i+1)*time.Minute))
		require.NoError(t, err)
		assert.Equal(t, i == 2, done)
	}
	assert.Equal(t, domain.StatusApproved, r.Status)
	assert.NotNil(t, r.DecidedAt)
	assert.Equal(t, hfa.UserID, *r.Steps[0].DecidedBy)

	_, err = r.Authorize(pd, nil)
	assert.Equal(t, apperr.CodeInvalidStateTransition, apperr.As(err).Code)
	_, err = r.Approve(pd, nil, "", t0)
	assert.Error(t, err)
}

func TestRejectRequiresNote(t *testing.T) {
	hfa := principal("head_finance")
	r := newPORequest(t, principal("purchasing"), "100")
	err := r.Reject(hfa, nil, "  ", t0)
	assert.Contains(t, apperr.As(err).Fields, "note")

	long := make([]rune, domain.MaxNoteLength+1)
	for i := range long {
		long[i] = 'a'
	}
	_, err = r.Approve(hfa, nil, string(long), t0)
	assert.Contains(t, apperr.As(err).Fields, "note")

	require.NoError(t, r.Reject(hfa, nil, "Harga terlalu tinggi", t0))
	assert.Equal(t, domain.StatusRejected, r.Status)
	assert.Equal(t, domain.DecisionRejected, *r.Steps[0].Decision)
	assert.Equal(t, "Harga terlalu tinggi", r.Steps[0].Note)
	assert.Error(t, r.Cancel(t0), "permintaan yang ditolak tidak bisa dibatalkan")
}

func TestRequesterNeverApprovesOwnDocument(t *testing.T) {
	acting := principal("head_finance", "purchasing")
	substitute := principal("finance")
	policies := poPolicies()
	policies[0].Substitute = &substitute.UserID
	s := subject(domain.SubjectPurchaseOrder, "100")
	r, err := domain.NewRequest(s, acting, domain.SelectPolicies(policies, s, t0), t0)
	require.NoError(t, err)
	assert.True(t, r.Steps[0].RequesterIsApprover)

	_, err = r.Authorize(acting, nil)
	assert.Equal(t, apperr.CodeForbidden, apperr.As(err).Code)

	from, err := r.Authorize(substitute, nil)
	require.NoError(t, err, "level naik ke pengganti")
	assert.Nil(t, from)

	other := principal("head_finance")
	_, err = r.Authorize(other, nil)
	assert.NoError(t, err, "pemegang jabatan lain tetap boleh")
}

func TestSubstituteOnlyWhenRequesterIsApprover(t *testing.T) {
	substitute := principal("finance")
	policies := poPolicies()
	policies[0].Substitute = &substitute.UserID
	s := subject(domain.SubjectPurchaseOrder, "100")
	r, err := domain.NewRequest(s, principal("purchasing"), domain.SelectPolicies(policies, s, t0), t0)
	require.NoError(t, err)
	_, err = r.Authorize(substitute, nil)
	assert.Error(t, err)
}

func TestDelegatedDecision(t *testing.T) {
	requester := principal("purchasing")
	hfa := principal("head_finance")
	delegate := principal("finance")
	r := newPORequest(t, requester, "100")

	_, err := r.Authorize(delegate, []domain.Principal{requester})
	assert.Error(t, err, "delegasi dari pembuat dokumen tidak memberi hak")

	from, err := r.Authorize(delegate, []domain.Principal{hfa})
	require.NoError(t, err)
	require.NotNil(t, from)
	assert.Equal(t, hfa.UserID, *from)

	done, err := r.Approve(delegate, from, "", t0)
	require.NoError(t, err)
	assert.True(t, done)
	assert.Equal(t, hfa.UserID, *r.Steps[0].DelegatedFrom)
}

func TestSpecificApproverAndDivisionScope(t *testing.T) {
	named := principal("head_finance")
	step := domain.Step{ApproverRole: "head_finance", ApproverUser: &named.UserID}
	assert.True(t, step.HoldsPosition(named))
	assert.False(t, step.HoldsPosition(principal("head_finance")), "hanya user yang ditunjuk")

	scoped := domain.Step{ApproverRole: "division_head", Division: &divLog}
	headLog := domain.Principal{UserID: uuid.New(), Roles: []domain.RoleGrant{{Code: "division_head", Division: &divLog}}}
	headSales := domain.Principal{UserID: uuid.New(), Roles: []domain.RoleGrant{{Code: "division_head", Division: &divSales}}}
	headAll := domain.Principal{UserID: uuid.New(), Roles: []domain.RoleGrant{{Code: "division_head"}}}
	assert.True(t, scoped.HoldsPosition(headLog))
	assert.False(t, scoped.HoldsPosition(headSales))
	assert.True(t, scoped.HoldsPosition(headAll))
}

func TestCancelAndReminder(t *testing.T) {
	r := newPORequest(t, principal("purchasing"), "100")
	assert.False(t, r.DueForReminder(t0.Add(59*time.Minute), domain.ReminderInterval))
	assert.True(t, r.DueForReminder(t0.Add(time.Hour), domain.ReminderInterval))
	reminded := t0.Add(time.Hour)
	r.CurrentStep().LastRemindedAt = &reminded
	assert.False(t, r.DueForReminder(t0.Add(90*time.Minute), domain.ReminderInterval))
	assert.True(t, r.DueForReminder(t0.Add(2*time.Hour), domain.ReminderInterval))

	require.NoError(t, r.Cancel(t0))
	assert.Equal(t, domain.StatusCancelled, r.Status)
	assert.False(t, r.DueForReminder(t0.Add(3*time.Hour), domain.ReminderInterval))
}

func TestInWorkingHours(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"Senin 08.00 WIB", time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC), true},
		{"Senin 07.59 WIB", time.Date(2026, 10, 19, 0, 59, 0, 0, time.UTC), false},
		{"Jumat 16.59 WIB", time.Date(2026, 10, 23, 9, 59, 0, 0, time.UTC), true},
		{"Jumat 17.00 WIB", time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC), false},
		{"Sabtu 10.00 WIB", time.Date(2026, 10, 24, 3, 0, 0, 0, time.UTC), false},
		{"Minggu 10.00 WIB", time.Date(2026, 10, 25, 3, 0, 0, 0, time.UTC), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { assert.Equal(t, tt.want, domain.InWorkingHours(tt.at)) })
	}
}

func TestDelegation(t *testing.T) {
	today := clock.NewBusinessDate(2026, 10, 19)
	from, to := uuid.New(), uuid.New()
	d, err := domain.NewDelegation(domain.DelegationInput{From: from, To: to, ValidFrom: today, ValidTo: today.AddDays(4), Reason: " Cuti "}, today)
	require.NoError(t, err)
	assert.Equal(t, "Cuti", d.Reason)
	assert.True(t, d.ActiveOn(today))
	assert.True(t, d.ActiveOn(today.AddDays(4)))
	assert.False(t, d.ActiveOn(today.AddDays(5)))

	require.NoError(t, d.Cancel(t0, today))
	assert.False(t, d.ActiveOn(today))
	assert.Error(t, d.Cancel(t0, today))

	ended := domain.Delegation{ValidFrom: today.AddDays(-5), ValidTo: today.AddDays(-1)}
	assert.Error(t, ended.Cancel(t0, today))

	long := make([]rune, 501)
	for i := range long {
		long[i] = 'x'
	}
	tests := []struct {
		name  string
		in    domain.DelegationInput
		field string
	}{
		{"tanpa penerima", domain.DelegationInput{From: from, ValidFrom: today, ValidTo: today}, "toUserId"},
		{"ke diri sendiri", domain.DelegationInput{From: from, To: from, ValidFrom: today, ValidTo: today}, "toUserId"},
		{"mulai kemarin", domain.DelegationInput{From: from, To: to, ValidFrom: today.AddDays(-1), ValidTo: today}, "validFrom"},
		{"tanpa tanggal", domain.DelegationInput{From: from, To: to}, "validTo"},
		{"selesai sebelum mulai", domain.DelegationInput{From: from, To: to, ValidFrom: today.AddDays(2), ValidTo: today.AddDays(1)}, "validTo"},
		{"lebih dari 90 hari", domain.DelegationInput{From: from, To: to, ValidFrom: today, ValidTo: today.AddDays(90)}, "validTo"},
		{"alasan panjang", domain.DelegationInput{From: from, To: to, ValidFrom: today, ValidTo: today, Reason: string(long)}, "reason"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewDelegation(tt.in, today)
			require.Error(t, err)
			assert.Contains(t, apperr.As(err).Fields, tt.field)
		})
	}
	_, err = domain.NewDelegation(domain.DelegationInput{From: from, To: to, ValidFrom: today, ValidTo: today.AddDays(89)}, today)
	assert.NoError(t, err, "tepat 90 hari diizinkan")
}

func TestSubjectTypes(t *testing.T) {
	types := domain.SubjectTypes()
	assert.Len(t, types, 15)
	info, ok := domain.LookupSubjectType(domain.SubjectReturnDisposition)
	require.True(t, ok)
	assert.True(t, info.HasCondition(domain.ConditionHasDisposal))
	assert.Equal(t, "Purchase Order", domain.SubjectPurchaseOrder.Label())
	assert.Equal(t, "lainnya", domain.SubjectType("lainnya").Label())
}
