import { describe, it, expect, afterEach } from "vitest";
import { render, cleanup, screen } from "@testing-library/react";
import { CommentCard } from "../components/CommentCard.js";

afterEach(cleanup);

describe("CommentCard", () => {
  it("renders the body as GitHub-flavored markdown, not raw text", () => {
    render(
      <CommentCard
        login="octocat"
        date="2026-01-01T00:00:00Z"
        body={"### Steps\n\n- [x] done\n\nSome `inline code` here."}
      />,
    );
    // The heading renders as an <h3>, not the literal "### Steps".
    const heading = screen.getByRole("heading", { level: 3, name: "Steps" });
    expect(heading).toBeInTheDocument();
    // Inline code renders as a <code> element.
    expect(screen.getByText("inline code").tagName).toBe("CODE");
    // The raw markdown tokens are not shown verbatim.
    expect(screen.queryByText(/### Steps/)).toBeNull();
  });

  it("shows a placeholder when the body is empty", () => {
    render(<CommentCard login="octocat" date="2026-01-01T00:00:00Z" body="" />);
    expect(screen.getByText("No description provided.")).toBeInTheDocument();
  });
});
