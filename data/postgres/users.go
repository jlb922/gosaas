package postgres

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/jlb922/gosaas/model"
)

type Users struct {
	DB *sql.DB
}

// StoreTempPassword stores the email and temp password
func (u *Users) StoreTempPassword(id int64, email, password string) error {
	//first delete any existing reset rows
	_, err := u.DB.Exec(`
		DELETE FROM gosaas_pwdreset
		WHERE (id = $1)
	`, id)

	_, err = u.DB.Exec(`
		INSERT INTO gosaas_pwdreset(id, email, password)
		VALUES($1, $2, $3)
	`, id, email, password)
	return err
}

func (u *Users) UpdateLastLogin(id int64) error {
	t := time.Now()
	_, err := u.DB.Exec(`
       UPDATE gosaas_users
       SET last_login = $1
       WHERE id = $2`, t, id)
	if err != nil {
		fmt.Println("error while uodating last login")
		return err
	}
	return nil
}

func (u *Users) SignUp(email, password, first, last string) (*model.Account, error) {
	var accountID int64

	err := u.DB.QueryRow(`
		INSERT INTO gosaas_accounts(
			email, 
			stripe_id, 
			subscription_id, 
			plan, 
			is_yearly, 
			subscribed_on, 
			seats,
			is_active
		)
		VALUES($1, '', '', '', false, $2, 0, true)
		RETURNING id
	`, email, time.Now()).Scan(&accountID)
	if err != nil {
		return nil, err
	}

	_, err = u.DB.Exec(`
		INSERT INTO gosaas_users(account_id, email, password, token, role, first, last)
		VALUES($1, $2, $3, $4, $5, $6, $7)
	`, accountID, email, password, model.NewToken(accountID), model.RoleAdmin, first, last)
	if err != nil {
		return nil, err
	}

	return u.GetDetail(accountID)
}

func (u *Users) Auth(accountID int64, token string, pat bool) (*model.Account, *model.User, error) {
	token = fmt.Sprintf("%d|%s", accountID, token)

	user := &model.User{}
	row := u.DB.QueryRow("SELECT id, account_id, first, last, email, password, token, role FROM gosaas_users WHERE account_id = $1 AND token = $2", accountID, token)
	if err := u.scanUser(row, user); err != nil {
		return nil, nil, err
	}

	account, err := u.GetDetail(user.AccountID)
	if err != nil {
		return nil, nil, err
	}

	return account, user, nil
}

func (u *Users) GetDetail(id int64) (*model.Account, error) {
	account := &model.Account{}
	row := u.DB.QueryRow("SELECT * FROM gosaas_accounts WHERE id = $1", id)
	err := row.Scan(&account.ID,
		&account.Email,
		&account.StripeID,
		&account.SubscriptionID,
		&account.Plan,
		&account.IsYearly,
		&account.SubscribedOn,
		&account.Seats,
		&account.IsActive,
	)
	if err != nil {
		fmt.Println("error while scanning account")
		return nil, err
	}

	//rows, err := u.DB.Query("SELECT * FROM gosaas_users WHERE account_id = $1", id)
	rows, err := u.DB.Query("SELECT id, account_id, first, last, email, password, token, role  FROM gosaas_users WHERE account_id = $1", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var user model.User
		if err := u.scanUser(rows, &user); err != nil {
			return nil, err
		}

		account.Users = append(account.Users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return account, nil
}

func (u *Users) GetUserByEmail(email string) (*model.User, error) {
	user := &model.User{}
	row := u.DB.QueryRow("SELECT id, account_id, first, last, email, password, token, role  FROM gosaas_users WHERE email = $1", email)
	if err := u.scanUser(row, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (u *Users) UserLogin(email string) error {
	return nil
}

func (u *Users) GetByStripe(stripeID string) (*model.Account, error) {
	var accountID int64
	row := u.DB.QueryRow("SELECT id FROM gosaas_accounts WHERE stripe_id = $1", stripeID)
	if err := row.Scan(&accountID); err != nil {
		return nil, err
	}

	return u.GetDetail(accountID)
}

func (repo *Users) ChangePassword(id, accountID int64, passwd string) error {
	_, err := repo.DB.Exec(`
	UPDATE gosaas_users
	SET password = $3
	WHERE id = $1
	AND account_id = $2
	`, id, accountID, passwd)
	return err
}

func (u *Users) SetSeats(id int64, seats int) error {
	_, err := u.DB.Exec(`
		UPDATE gosaas_accounts SET
			seats = $2
		WHERE id = $1
	`, id, seats)
	return err
}

func (u *Users) ConvertToPaid(id int64, stripeID, subID, plan string, yearly bool, seats int) error {
	_, err := u.DB.Exec(`
		UPDATE gosaas_accounts SET
			stripe_id = $2,
			subscription_id = $3,
			subscribed_on = $4,
			plan = $5,
			seats = $6,
			is_yearly = $7
		WHERE id = $1
	`, id, stripeID, subID, time.Now(), plan, seats, yearly)
	return err
}

func (u *Users) ChangePlan(id int64, plan string, yearly bool) error {
	_, err := u.DB.Exec(`
		UPDATE gosaas_accounts SET
			plan = $2,
			is_yearly = $3
		WHERE id = $1
	`, id, plan, yearly)
	return err
}

func (u *Users) Cancel(id int64) error {
	_, err := u.DB.Exec(`
		UPDATE gosaas_accounts SET
			subscription_id = '',
			plan = '',
			is_yearly = false
		WHERE id = $1
	`, id)
	return err
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func (u *Users) AddToken(accountID, userID int64, name string) (*model.AccessToken, error) {
	return nil, fmt.Errorf("not implemented")
}

func (u *Users) RemoveToken(accountID, userID, tokenID int64) error {
	return fmt.Errorf("not implemented")
}

func (u *Users) scanUser(rows scanner, user *model.User) error {
	//var str *string
	return rows.Scan(&user.ID,
		&user.AccountID,
		&user.First,
		&user.Last,
		&user.Email,
		&user.Password,
		&user.Token,
		&user.Role,
		//&str,
	)
}
