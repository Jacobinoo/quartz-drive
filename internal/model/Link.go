package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Link struct {
	// ID: Unikalne ID linku, nie mylić z ID pliku/węzła
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index:idx_parent_node_deleted"`

	// ParentNodeID: Wskazuje na FOLDER (Węzeł), w którym ten link się znajduje.
	// - Jeśli != NULL: To jest zwykły plik/folder wewnątrz innego folderu.
	//   (Szyfrowane kluczem ParentNode).
	// - Jeśli == NULL: To jest "Volume Root" (główny folder udziału).
	//   (Szyfrowane kluczem Share).
	ParentNodeID *uuid.UUID `gorm:"type:uuid;index:idx_parent_node_deleted"`

	// ChildNodeID: Wskazuje na PLIK lub FOLDER (Węzeł), który ten link reprezentuje.
	// To tutaj "żyje" plik. Link to tylko etykieta w katalogu rodzica.
	ChildNodeID *uuid.UUID `gorm:"type:uuid;index;not null"`

	// EncryptedName: Nazwa pliku ("wakacje.jpg").
	// Klucz szyfrujący:
	//   - ParentNode.PrivKey (gdy ParentNodeID != NULL)
	//   - Share.Key (gdy ParentNodeID == NULL)
	EncryptedName string `gorm:"type:text;not null"`
	NameNonce     string `gorm:"type:text;not null"`

	// EncryptedNodePassphrase: To jest "klucz do sejfu" (hasło węzła ChildNode).
	// Dzięki temu, mając dostęp do Rodzica, automatycznie otwierasz Dziecko.
	// Algorytm: crypto_box_seal (Asymetryczne) LUB crypto_aead (Symetryczne) - zależnie od implementacji.
	//   W Protonie/naszym modelu: Zamykamy passphrase dziecka w "pudełku" (Sealed Box)
	//   otwieranym przez klucz prywatny Rodzica.
	EncryptedNodePassphrase string `gorm:"type:text;not null"`

	SignedEncryptedNodePassphrase string `gorm:"type:text;not null"`

	AuthorID uuid.UUID `gorm:"type:uuid;not null"`

	ParentNode *Node `gorm:"foreignKey:ParentNodeID;constraint:OnDelete:CASCADE"`
	ChildNode  Node  `gorm:"foreignKey:ChildNodeID;constraint:OnDelete:CASCADE"`
	Author     User  `gorm:"foreignKey:AuthorID"`
}
