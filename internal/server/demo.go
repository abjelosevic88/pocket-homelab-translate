package server

import (
	"context"
	"math/rand"
	"time"

	"github.com/abjelosevic88/pocket-homelab-translate/internal/docs"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/store"
)

// seedDemo fills the owner's account with a few months of translating:
// history, a phrasebook in collections, a glossary, conversations and
// translated documents (DEMO_DATA, for trying it out and screenshots).
func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	ctx = context.WithoutCancel(ctx)
	if _, _, done := s.store.CacheGet(ctx, "demo:seeded"); done {
		return nil
	}
	if n, _ := s.store.StatsFor(ctx, uid, store.Today(), store.Today()); n.Total > 0 {
		return nil
	}
	rnd := rand.New(rand.NewSource(42))
	now := time.Now()
	at := func(daysAgo int) string {
		t := now.AddDate(0, 0, -daysAgo).Add(-time.Duration(rnd.Intn(10*60)) * time.Minute)
		return t.UTC().Format(time.RFC3339Nano)
	}
	type phrase struct{ from, to, src, res, coll, note string }
	starred := []phrase{
		{"en", "it", "Could we have the bill, please?", "Potremmo avere il conto, per favore?", "Travel", ""},
		{"en", "it", "Is this seat free?", "È libero questo posto?", "Travel", ""},
		{"en", "it", "A table for four, please.", "Un tavolo per quattro, per favore.", "Restaurant", ""},
		{"en", "it", "I am allergic to nuts.", "Sono allergico alla frutta a guscio.", "Restaurant", "Say it before ordering"},
		{"en", "it", "Where is the train station?", "Dov'è la stazione ferroviaria?", "Travel", ""},
		{"en", "it", "Two tickets to Florence, please.", "Due biglietti per Firenze, per favore.", "Travel", ""},
		{"en", "de", "I'd like to make an appointment.", "Ich möchte einen Termin vereinbaren.", "Doctor", ""},
		{"en", "de", "It hurts here.", "Es tut hier weh.", "Doctor", ""},
		{"en", "de", "Do I need a prescription for this?", "Brauche ich dafür ein Rezept?", "Doctor", ""},
		{"en", "de", "Could you send me the invoice by email?", "Könnten Sie mir die Rechnung per E-Mail schicken?", "Work", ""},
		{"en", "de", "Let's move the meeting to Thursday.", "Verschieben wir das Meeting auf Donnerstag.", "Work", ""},
		{"en", "de", "Thank you for your patience.", "Vielen Dank für Ihre Geduld.", "Work", "Formal"},
		{"sr-Latn", "en", "Javiću ti se čim stignem.", "I'll get in touch as soon as I arrive.", "Everyday", ""},
		{"sr-Latn", "en", "Nema na čemu.", "You're welcome.", "Everyday", ""},
		{"sr-Latn", "en", "Samo polako, imamo vremena.", "Take it easy, we have time.", "Everyday", ""},
		{"en", "sr-Latn", "The deadline is end of the month.", "Rok je kraj meseca.", "Work", ""},
		{"en", "sr-Latn", "Break a leg!", "Srećno!", "Everyday", "An idiom: wishing good luck"},
		{"en", "fr", "Where can I buy a SIM card?", "Où puis-je acheter une carte SIM ?", "Travel", ""},
		{"en", "fr", "Is breakfast included?", "Le petit-déjeuner est-il inclus ?", "Travel", ""},
		{"en", "es", "How much is it?", "¿Cuánto cuesta?", "Travel", ""},
	}
	history := []phrase{
		{"en", "sr-Latn", "Please find the signed contract attached.", "U prilogu vam šaljem potpisan ugovor.", "", ""},
		{"de", "en", "Die Lieferung verzögert sich um zwei Tage.", "The delivery is delayed by two days.", "", ""},
		{"en", "de", "We are looking forward to working with you.", "Wir freuen uns auf die Zusammenarbeit mit Ihnen.", "", ""},
		{"it", "en", "Chiuso per ferie fino al 25 agosto.", "Closed for holidays until 25 August.", "", ""},
		{"en", "sr-Cyrl", "Happy birthday, grandma!", "Срећан рођендан, бако!", "", ""},
		{"sr-Latn", "de", "Molim vas, gde je najbliža apoteka?", "Entschuldigung, wo ist die nächste Apotheke?", "", ""},
		{"en", "it", "What time does the museum open?", "A che ora apre il museo?", "", ""},
		{"fr", "en", "Le colis sera livré demain matin.", "The parcel will be delivered tomorrow morning.", "", ""},
		{"en", "de", "The heating in the flat is not working.", "Die Heizung in der Wohnung funktioniert nicht.", "", ""},
		{"de", "sr-Latn", "Bitte bringen Sie Ihren Ausweis mit.", "Molimo vas da ponesete ličnu kartu.", "", ""},
		{"en", "sr-Latn", "Can you recommend a good dentist?", "Možeš li da mi preporučiš dobrog zubara?", "", ""},
		{"es", "en", "La reunión empieza a las diez.", "The meeting starts at ten.", "", ""},
		{"en", "it", "Is there a pharmacy nearby?", "C'è una farmacia qui vicino?", "", ""},
		{"nl", "en", "Bedankt voor je snelle reactie.", "Thanks for your quick reply.", "", ""},
		{"en", "de", "Sorry for the late reply.", "Entschuldigen Sie die späte Antwort.", "", ""},
		{"sr-Latn", "en", "Stižem za deset minuta.", "I'll be there in ten minutes.", "", ""},
	}
	engines := []string{"ai", "ai", "ai", "nllb", "argos"}
	kinds := []string{"text", "text", "text", "text", "photo", "speech"}
	for i, p := range starred {
		t := &store.Translation{Kind: "text", SourceLang: p.from, DetectedLang: p.from, TargetLang: p.to, SourceText: p.src, ResultText: p.res,
			Engine: engines[rnd.Intn(len(engines))], Starred: true, Collection: p.coll, Note: p.note, CreatedAt: at(5 + i*4 + rnd.Intn(4))}
		if i%3 == 0 {
			t.LearnRef = "added"
		}
		if _, err := s.store.AddTranslation(ctx, uid, t); err != nil {
			return err
		}
	}
	for round := 0; round < 9; round++ {
		for i, p := range history {
			if rnd.Intn(3) == 0 {
				continue
			}
			t := &store.Translation{Kind: kinds[rnd.Intn(len(kinds))], SourceLang: "auto", DetectedLang: p.from, TargetLang: p.to, SourceText: p.src, ResultText: p.res,
				Engine: engines[rnd.Intn(len(engines))], CreatedAt: at(round*12 + i%12 + 1)}
			if t.Engine == "ai" {
				t.Model = "pocket"
			}
			if _, err := s.store.AddTranslation(ctx, uid, t); err != nil {
				return err
			}
		}
	}
	// today
	for i, p := range history[:4] {
		t := &store.Translation{Kind: "text", SourceLang: "auto", DetectedLang: p.from, TargetLang: p.to, SourceText: p.src, ResultText: p.res, Engine: "ai", Model: "pocket",
			CreatedAt: now.Add(-time.Duration(20+i*35) * time.Minute).UTC().Format(time.RFC3339Nano)}
		_, _ = s.store.AddTranslation(ctx, uid, t)
	}

	terms := []store.Term{
		{SourceLang: "en", TargetLang: "sr-Latn", Term: "invoice", Translation: "faktura", Note: "not \"račun\" at work"},
		{SourceLang: "en", TargetLang: "sr-Latn", Term: "deadline", Translation: "rok"},
		{SourceLang: "en", TargetLang: "sr-Latn", Term: "Pocket", Translation: "Pocket", Note: "the product name stays"},
		{SourceLang: "en", TargetLang: "de", Term: "invoice", Translation: "Rechnung"},
		{SourceLang: "en", TargetLang: "de", Term: "meeting", Translation: "Meeting", Note: "the team says Meeting, not Besprechung"},
		{SourceLang: "en", TargetLang: "de", Term: "board", Translation: "Vorstand", Note: "the company's board"},
		{SourceLang: "en", TargetLang: "de", Term: "Athena Studio", Translation: "Athena Studio", MatchCase: true},
		{SourceLang: "*", TargetLang: "en", Term: "Bjelke", Translation: "Bjelke", MatchCase: true, Note: "a name"},
		{SourceLang: "de", TargetLang: "sr-Latn", Term: "Ausweis", Translation: "lična karta"},
		{SourceLang: "en", TargetLang: "it", Term: "gluten-free", Translation: "senza glutine"},
	}
	for i := range terms {
		_ = s.store.AddTerm(ctx, uid, &terms[i])
	}

	type turn struct{ side, src, res string }
	convs := []struct {
		title, a, b string
		days        int
		turns       []turn
	}{
		{"Pharmacy in Florence", "en", "it", 18, []turn{
			{"a", "Hello, I need something for a sore throat.", "Buongiorno, ho bisogno di qualcosa per il mal di gola."},
			{"b", "Ha anche la febbre?", "Do you also have a fever?"},
			{"a", "No, just the throat. It started yesterday.", "No, solo la gola. È iniziato ieri."},
			{"b", "Allora le consiglio queste pastiglie, tre volte al giorno.", "Then I recommend these lozenges, three times a day."},
			{"a", "Thank you. How much is it?", "Grazie. Quanto costa?"},
			{"b", "Sono otto euro e cinquanta.", "That's eight euros fifty."},
		}},
		{"Landlord about the heating", "sr-Latn", "de", 6, []turn{
			{"a", "Dobar dan, grejanje u stanu ne radi od jutros.", "Guten Tag, die Heizung in der Wohnung funktioniert seit heute Morgen nicht."},
			{"b", "Oh, das tut mir leid. Ich schicke morgen einen Techniker.", "Oh, žao mi je. Sutra ću poslati tehničara."},
			{"a", "U koliko sati otprilike?", "Um wie viel Uhr ungefähr?"},
			{"b", "Zwischen neun und zwölf Uhr.", "Između devet i dvanaest sati."},
		}},
		{"Taxi from the airport", "en", "fr", 40, []turn{
			{"a", "To this address, please.", "À cette adresse, s'il vous plaît."},
			{"b", "D'accord. Environ quarante minutes avec la circulation.", "Okay. About forty minutes with the traffic."},
			{"a", "Can I pay by card?", "Je peux payer par carte ?"},
			{"b", "Oui, bien sûr.", "Yes, of course."},
		}},
	}
	for _, cv := range convs {
		c := &store.Conversation{Title: cv.title, LangA: cv.a, LangB: cv.b, CreatedAt: at(cv.days)}
		c.UpdatedAt = c.CreatedAt
		if err := s.store.AddConversation(ctx, uid, c); err != nil {
			return err
		}
		base, _ := time.Parse(time.RFC3339Nano, c.CreatedAt)
		for i, t := range cv.turns {
			from, to := cv.a, cv.b
			if t.side == "b" {
				from, to = cv.b, cv.a
			}
			tr := &store.Turn{ConversationID: c.ID, Side: t.side, Spoken: i%3 != 2, SourceLang: from, TargetLang: to, SourceText: t.src, ResultText: t.res,
				Engine: "ai", CreatedAt: base.Add(time.Duration(i*40) * time.Second).Format(time.RFC3339Nano)}
			_ = s.store.AddTurn(ctx, uid, tr)
		}
	}

	// documents, already translated
	ddocs := []struct {
		name, from, to string
		days           int
		src, res       []string
		failed         string
	}{
		{"Rental agreement.docx", "de", "sr-Latn", 9,
			[]string{"Mietvertrag", "Die Wohnung wird ab dem 1. Dezember vermietet.", "Die Kaution beträgt drei Monatsmieten.", "Haustiere sind nach Absprache erlaubt."},
			[]string{"Ugovor o zakupu", "Stan se izdaje od 1. decembra.", "Depozit iznosi tri mesečne kirije.", "Kućni ljubimci su dozvoljeni uz dogovor."}, ""},
		{"Project brief.docx", "en", "de", 3,
			[]string{"Project brief", "We are building a booking page for a small dental practice.", "The first version should be ready by the end of the month."},
			[]string{"Projektbeschreibung", "Wir bauen eine Buchungsseite für eine kleine Zahnarztpraxis.", "Die erste Version soll bis Ende des Monats fertig sein."}, ""},
		{"Doctor's report.pdf", "it", "en", 17,
			[]string{"Referto medico", "Il paziente presenta una lieve infiammazione della gola.", "Si consiglia riposo e molti liquidi."},
			[]string{"Medical report", "The patient has a mild inflammation of the throat.", "Rest and plenty of fluids are recommended."}, ""},
		{"Old scan.pdf", "auto", "en", 1, nil, nil, "This PDF is a scan and reading scans needs tesseract on this server."},
	}
	for _, dd := range ddocs {
		orig := []byte("%PDF-1.4 demo")
		if dd.src != nil {
			orig, _ = docs.NewDocx(dd.src, "")
		}
		d := &store.Document{Name: dd.name, Format: docs.FormatOf(dd.name, ""), Mime: docs.Mime(".docx"), SourceLang: dd.from, TargetLang: dd.to, Engine: "auto", Origin: "upload", Status: store.DocWorking}
		if d.Format == "pdf" {
			d.Mime = "application/pdf"
		}
		// stored as done (or failed) straight away, so the worker leaves it
		if err := s.store.AddDocument(ctx, uid, d, orig); err != nil {
			return err
		}
		_, _ = s.store.DB.ExecContext(ctx, `UPDATE documents SET created_at = ? WHERE id = ?`, at(dd.days), d.ID)
		if dd.failed != "" {
			d.Status, d.Error = store.DocFailed, dd.failed
			_ = s.store.FinishDocument(ctx, d, nil)
			continue
		}
		res, _ := docs.NewDocx(dd.res, "")
		d.Status, d.Used, d.Done, d.Total, d.DetectedLang = store.DocDone, "ai", len(dd.src), len(dd.src), dd.from
		d.ResultName = docs.ResultName(dd.name, dd.to, ".docx")
		_ = s.store.FinishDocument(ctx, d, res)
	}
	st, _ := s.store.GetSettings(ctx, uid)
	st.TargetLang, st.RecentLangs = "sr-Latn", []string{"sr-Latn", "en", "de", "it", "fr"}
	_ = s.store.SaveUserSettings(ctx, uid, st)
	_ = s.store.CachePut(ctx, "demo:seeded", "1")
	s.log.Info("demo data added", "user", uid)
	return nil
}
