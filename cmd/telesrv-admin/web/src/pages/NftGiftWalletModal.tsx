import { Wallet } from "lucide-react";
import { useState } from "react";
import { CommandFlow } from "../components/CommandFlow";
import { Alert } from "../components/ui";
import { useI18n } from "../i18n";
import type { NftGiftWalletPayload, UniqueStarGiftRow } from "../types";

type Mode = "bind" | "release";

// NftGiftWalletModal is the operator form behind "any gift -> any TON wallet":
// it addresses a collectible by slug, NFT address or id and fills the owner
// wallet record by hand, edits an existing wallet binding, or releases the gift
// back to its Telegram owner. TON owner, Telegram holder and host stay separate
// fields here, mirroring the ledger.
//
// The reason -> dry-run -> confirm flow is CommandFlow's, shared with the gift
// pack importer. That is the point: this modal used to reimplement it, which is
// how the audit reason -- the one field every command in this panel is required
// to carry -- ended up as the piece a hand-rolled copy was most likely to
// diverge on.
export function NftGiftWalletModal({
  giftRef,
  gift,
  onClose,
  onApplied,
}: {
  giftRef: string;
  gift?: UniqueStarGiftRow | null;
  onClose: () => void;
  onApplied: () => void;
}) {
  const { t } = useI18n();
  const bound = Boolean(gift?.OwnerAddress);
  const [ref, setRef] = useState(giftRef);
  const [mode, setMode] = useState<Mode>("bind");
  const [ownerName, setOwnerName] = useState(gift?.WalletName ?? "");
  const [walletAddress, setWalletAddress] = useState(gift?.OwnerAddress ?? "");
  const [hostUserID, setHostUserID] = useState(
    gift?.HostPeerType === "user" && gift.HostPeerID !== "0" ? gift.HostPeerID : "",
  );

  // The payload is derived from the form rather than assembled at submit time,
  // so the request preview in the modal shows the operator exactly what is
  // about to be sent, and CommandFlow can send it again on confirm.
  function buildPayload(): NftGiftWalletPayload {
    const payload: NftGiftWalletPayload = {
      command_id: "",
      reason: "",
      confirm: false,
      ref: ref.trim(),
      clear: mode === "release",
    };
    if (mode === "bind") {
      payload.wallet_name = ownerName.trim();
      payload.wallet_address = walletAddress.trim();
      if (hostUserID.trim()) payload.host_user_id = hostUserID.trim();
    }
    return payload;
  }

  const modeButton = (value: Mode, text: string) => (
    <button
      className={`btn ${mode === value ? "primary" : ""}`}
      type="button"
      onClick={() => setMode(value)}
    >
      {text}
    </button>
  );

  return (
    <CommandFlow
      path="/api/actions/set-nft-gift-wallet"
      label={t("nft.walletTitle")}
      onApplied={onApplied}
      onClose={onClose}
      payload={() => buildPayload()}
    >
      {gift && (
        <div className="gift-validation">
          <div className="gift-validation-head">
            <Wallet size={17} />
            <div>
              <strong>{t("nft.walletCurrent")}</strong>
              <span>
                {gift.Title || gift.Slug} #{gift.Num} —{" "}
                {bound
                  ? `${gift.WalletName || t("nft.ownerTon")}: ${gift.OwnerAddress}`
                  : t("nft.walletNone")}
              </span>
              {gift.HostPeerID !== "0" && (
                <span>{`${t("nft.hostedBy")} ${gift.HostPeerType}/${gift.HostPeerID}`}</span>
              )}
            </div>
          </div>
        </div>
      )}
      <div className="bot-create-fields">
        {modeButton("bind", bound ? t("nft.walletEdit") : t("nft.walletBind"))}
        {modeButton("release", t("nft.walletRelease"))}
      </div>
      <div className="gift-fields-grid">
        <label>
          <span>{t("nft.walletRef")}</span>
          <input value={ref} onChange={(event) => setRef(event.target.value)} placeholder="slug / EQ… / 9000000000000001" />
        </label>
      </div>
      {mode === "bind" ? (
        <div className="gift-fields-grid">
          <label>
            <span>{t("nft.walletOwnerName")}</span>
            <input
              aria-label={t("nft.walletOwnerName")}
              maxLength={64}
              placeholder={t("nft.walletNamePlaceholder")}
              value={ownerName}
              onChange={(event) => setOwnerName(event.target.value)}
            />
          </label>
          <label>
            <span>{t("nft.walletAddress")}</span>
            <input
              aria-label={t("nft.walletAddress")}
              value={walletAddress}
              onChange={(event) => setWalletAddress(event.target.value)}
              placeholder={t("nft.walletAddressPlaceholder")}
            />
          </label>
          <label>
            <span>{t("nft.walletHost")}</span>
            <input
              aria-label={t("nft.walletHost")}
              value={hostUserID}
              inputMode="numeric"
              placeholder="0"
              onChange={(event) => setHostUserID(event.target.value)}
            />
          </label>
        </div>
      ) : (
        <Alert>{t("nft.walletReleaseNote")}</Alert>
      )}
    </CommandFlow>
  );
}