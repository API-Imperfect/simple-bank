package db

import (
	"context"
	"database/sql"
	"fmt"
)

type Store interface {
	Querier
	TransferTx(ctx context.Context, arg TransferTxParams) (TransferTxResult, error)
}

type SQLStore struct {
	*Queries
	db *sql.DB
}

func NewStore(db *sql.DB) Store {
	return &SQLStore{
		db:      db,
		Queries: New(db),
	}
}

func (store *SQLStore) execTx(ctx context.Context, fn func(*Queries) error) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	q := New(tx)

	err = fn(q)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx err: %v, rb err: %v", err, rbErr)
		}
		return err
	}
	return tx.Commit()
}

type TransferTxParams struct {
	FromAccountID int64 `json:"from_account_id"`
	ToAccountID   int64 `json:"to_account_id"`
	Amount        int64 `json:"amount"`
}

type TransferTxResult struct {
	Transfer    Transfer `json:"transfer"`
	FromAccount Account  `json:"from_account"`
	ToAccount   Account  `json:"to_account"`
	FromEntry   Entry    `json:"from_entry"`
	ToEntry     Entry    `json:"to_entry"`
}

func (store *SQLStore) TransferTx(ctx context.Context, arg TransferTxParams) (TransferTxResult, error) {
	// Input validation
	if err := validateTransferParams(arg); err != nil {
		return TransferTxResult{}, err
	}

	var result TransferTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		// Create transfer record
		result.Transfer, err = q.CreateTransfer(ctx, CreateTransferParams(arg))
		if err != nil {
			return err
		}

		// Create entry records
		result.FromEntry, err = q.CreateEntry(ctx, CreateEntryParams{
			AccountID: arg.FromAccountID,
			Amount:    -arg.Amount,
		})
		if err != nil {
			return err
		}

		result.ToEntry, err = q.CreateEntry(ctx, CreateEntryParams{
			AccountID: arg.ToAccountID,
			Amount:    arg.Amount,
		})
		if err != nil {
			return err
		}

		// Update account balances with proper locking order
		result.FromAccount, result.ToAccount, err = updateAccountBalances(ctx, q, arg)
		return err
	})

	return result, err
}

// validateTransferParams validates the transfer parameters
func validateTransferParams(arg TransferTxParams) error {
	if arg.FromAccountID == arg.ToAccountID {
		return fmt.Errorf("cannot transfer to the same account")
	}
	if arg.Amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	return nil
}

// updateAccountBalances updates account balances with proper locking order to prevent deadlocks
func updateAccountBalances(ctx context.Context, q *Queries, arg TransferTxParams) (Account, Account, error) {
	// Determine lock order based on account IDs to prevent deadlocks
	firstAccountID, secondAccountID := arg.FromAccountID, arg.ToAccountID
	if firstAccountID > secondAccountID {
		firstAccountID, secondAccountID = secondAccountID, firstAccountID
	}

	// Lock accounts in consistent order
	firstAccount, err := q.GetAccountForUpdate(ctx, firstAccountID)
	if err != nil {
		return Account{}, Account{}, err
	}

	secondAccount, err := q.GetAccountForUpdate(ctx, secondAccountID)
	if err != nil {
		return Account{}, Account{}, err
	}

	// Check if from account has sufficient balance
	if firstAccountID == arg.FromAccountID && firstAccount.Balance < arg.Amount {
		return Account{}, Account{}, fmt.Errorf("insufficient balance: account %d has %d, trying to transfer %d",
			arg.FromAccountID, firstAccount.Balance, arg.Amount)
	}
	if secondAccountID == arg.FromAccountID && secondAccount.Balance < arg.Amount {
		return Account{}, Account{}, fmt.Errorf("insufficient balance: account %d has %d, trying to transfer %d",
			arg.FromAccountID, secondAccount.Balance, arg.Amount)
	}

	// Update balances using AddAccountBalance
	fromAccount, err := q.AddAccountBalance(ctx, AddAccountBalanceParams{
		ID:     arg.FromAccountID,
		Amount: -arg.Amount,
	})
	if err != nil {
		return Account{}, Account{}, err
	}

	toAccount, err := q.AddAccountBalance(ctx, AddAccountBalanceParams{
		ID:     arg.ToAccountID,
		Amount: arg.Amount,
	})
	if err != nil {
		return Account{}, Account{}, err
	}

	return fromAccount, toAccount, nil
}
