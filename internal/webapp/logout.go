package webapp

import (
	"net/http"
	"time"
)

func (api *Api) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Secure:   true,
		HttpOnly: true,
		Name:     "GSESS",
		Value:    "",
		Path:     "/func",
		Expires:  time.Unix(0, 0),
	})
	sqlStmt := `DELETE FROM session WHERE session_id = $1`
	ck, err := r.Cookie("GSESS")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ct, err := api.db.Exec(r.Context(), sqlStmt, ck.Value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if ct.RowsAffected() != 1 {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}
