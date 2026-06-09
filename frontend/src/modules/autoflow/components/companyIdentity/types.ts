// Company Identity form shape — mirror of the backend company-identity DTO
// (GET/PUT /api/org/company-identity). Every field is optional; an empty field
// means the agent must NOT mention it (no fabrication).
export interface CompanyIdentity {
  company_name: string;
  website: string;
  official_contact: string;
  primary_cta: string;
  service_summary: string;
}

export const EMPTY_COMPANY_IDENTITY: CompanyIdentity = {
  company_name: '',
  website: '',
  official_contact: '',
  primary_cta: '',
  service_summary: '',
};
