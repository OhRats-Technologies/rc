import type { ReactNode } from "react";

export type DocSection = {
  id: string;
  title: string;
  body: ReactNode;
};

export type DocArticle = {
  slug: string;
  title: string;
  intro: string;
  sections: DocSection[];
  copy?: boolean;
};
