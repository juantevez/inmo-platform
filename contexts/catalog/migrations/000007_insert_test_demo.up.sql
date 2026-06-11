INSERT INTO profiles (user_id, first_name, last_name, dni_cuit, phone, profile_type, company_name, license_number, status)
VALUES (
    'bc867558-0e4b-a8f8-5028-7fc25cc146a2',
    'Juan', 'Tevez', '20-38472918-9', '+541165849321',
    'COMMERCIAL', 'Tevez Propiedades', 'CUCICBA Matricula N° 4821', 'ACTIVE'
) ON CONFLICT (user_id) DO NOTHING;

INSERT INTO profiles (user_id, first_name, last_name, dni_cuit, phone, profile_type, company_name, license_number, status)
VALUES (
    '46b1cd7d-b1f9-b3ae-d047-8ce4d9ebf664',
    'Diego', 'Maradona', '20-10301030-0', '+541110301030',
    'INDIVIDUAL', NULL, NULL, 'ACTIVE'
) ON CONFLICT (user_id) DO NOTHING;
