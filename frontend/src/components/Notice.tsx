import { Check, X } from "lucide-react";

function Notice({ text, kind = "error" }: { text: string; kind?: "error" | "success" }) {
  return (
    <div className={`notice ${kind}`} role={kind === "error" ? "alert" : "status"}>
      {kind === "error" ? <X aria-hidden="true" /> : <Check aria-hidden="true" />}
      {text}
    </div>
  );
}

export default Notice;
