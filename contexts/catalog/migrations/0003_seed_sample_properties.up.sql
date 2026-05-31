TRUNCATE TABLE properties CASCADE;

INSERT INTO properties (id, owner_id, title, description, price, currency, latitude, longitude, address, state) VALUES
-- 🏢 DEPARTAMENTOS EN ALQUILER
('PROP-001', 'owner-999', 'Depto 2 Ambientes Luminoso en Adrogué', 'Hermoso departamento de 45m2 al frente con balcón corrido, 1 dormitorio, cocina integrada y baño completo. Excelente luminosidad natural.', 450000.00, 'ARS', -34.798100, -58.389200, 'Mitre 450', 'AVAILABLE'),
('PROP-002', 'owner-999', 'Monoambiente Moderno Divisible', 'Excelente ubicación céntrica en Ramos Mejía, ideal estudiantes o profesionales. 32m2 totales, amenities en terraza y bajas expensas.', 380000.00, 'ARS', -34.643200, -58.562100, 'Av. Rivadavia 14200', 'AVAILABLE'),
('PROP-003', 'owner-999', 'Semi-piso 3 Ambientes con Cochera', 'Edificio de categoría en Recoleta. 85m2, seguridad 24hs, suite con vestidor, dos baños, calefacción por losa radiante y cochera fija.', 850.00, 'USD', -34.588500, -58.397400, 'Las Heras 2300', 'AVAILABLE'),
('PROP-004', 'owner-999', 'Departamento 4 Ambientes Familiar', 'Amplio living comedor en Ramos Mejía. Tres dormitorios con placard, cocina comedor diario, dos baños y lavadero independiente. 110m2.', 700000.00, 'ARS', -34.645500, -58.565000, 'Avenida de Mayo 800', 'AVAILABLE'),
('PROP-005', 'owner-999', 'Duplex con Terraza Propia', 'Moderno diseño de dos plantas en Adrogué. 3 ambientes, 2 dormitorios, parrilla individual en terraza privada. 75m2 totales.', 550000.00, 'ARS', -34.799500, -58.391000, 'Seguí 150', 'AVAILABLE'),

-- 🏡 CASAS EN ALQUILER
('PROP-006', 'owner-999', 'Chalet Tradicional de Categoría', 'Desarrollada en dos plantas en zona residencial de Adrogué. 4 ambientes, gran jardín con piscina, quincho cerrado y cochera para dos autos. 220m2.', 1500.00, 'USD', -34.796500, -58.385500, 'Spiro 300', 'AVAILABLE'),
('PROP-007', 'owner-999', 'Casa de Estilo Americano', 'Living con hogar a leña en Ramos Mejía. Cocina comedor, 2 dormitorios, entrada de auto, patio mediano con parrilla. Sólida estructura de 130m2.', 850000.00, 'ARS', -34.641000, -58.559500, 'Rosales 1400', 'AVAILABLE'),
('PROP-008', 'owner-999', 'Casa Quinta Minimalista', 'Ideal fin de semana o vivienda permanente en El Peligro. Parque arbolado de 1000m2, piscina climatizada, 4 dormitorios, 4 baños. 310m2 cubiertos.', 2200.00, 'USD', -34.912000, -58.154000, 'Ruta 2 Km 45', 'AVAILABLE'),

-- 🏢 DEPARTAMENTOS EN VENTA
('PROP-009', 'owner-999', 'Depto 2 Ambientes en Pozo', 'Oportunidad de inversión en Ramos Mejía. Emprendimiento en pozo, excelente financiación, entrega en 12 meses. 42m2 con balcón.', 65000.00, 'USD', -34.644000, -58.561000, 'Alsina 800', 'AVAILABLE'),
('PROP-010', 'owner-999', '3 Ambientes Reciclado a Nuevo', 'Cañerías e instalación eléctrica a estrenar en Palermo. Pisos de parqué pulidos y plastificados, cocina equipada de primera calidad. 68m2.', 115000.00, 'USD', -34.582200, -58.417800, 'Avenida Santa Fe 3400', 'AVAILABLE'),
('PROP-011', 'owner-999', 'Penthouse Exclusivo Vista al Río', 'Piso alto de súper lujo en Vicente López. Terraza perimetral, jacuzzi privado, 3 dormitorios en suite, cochera doble fija y baulera. 190m2.', 420000.00, 'USD', -34.529000, -58.472000, 'Av. Del Libertador 1500', 'AVAILABLE'),
('PROP-012', 'owner-999', 'Monoambiente Amplio Centro', 'Apto crédito bancario en Ramos Mejía centro. 30m2 totales, bajas expensas. Excelente oportunidad para renta temporal o primera vivienda.', 48000000.00, 'ARS', -34.642800, -58.563200, 'Cordoba 1200', 'AVAILABLE'),
('PROP-013', 'owner-999', '2 Ambientes con Amenities de Lujo', 'Ubicado en La Plata. Edificio con piscina, SUM, gimnasio, laundry y seguridad. Departamento de 52m2 con balcón y parrilla eléctrica.', 135000.00, 'USD', -34.921200, -57.954500, 'Plaza Paso 45', 'AVAILABLE'),
('PROP-014', 'owner-999', 'Depto 3 Ambientes Frente a la Plaza', 'Excelente luminosidad natural en Adrogué centro. 2 dormitorios, 2 baños, cocina con barra desayunadora y cochera subterránea. 70m2.', 98000.00, 'USD', -34.797200, -58.388800, 'Macias 200', 'AVAILABLE'),

-- 🏡 CASAS EN VENTA
('PROP-015', 'owner-999', 'Casa Moderna en Barrio Cerrado', 'Lote central en Canning. Arquitectura minimalista, aberturas de aluminio con DVH, cocina con isla, 3 dormitorios (1 en suite con jacuzzi). 260m2.', 340000.00, 'USD', -34.875000, -58.504000, 'Barrio Las Alondras Lote 42', 'AVAILABLE'),
('PROP-016', 'owner-999', 'Chalet Inglés Histórico', 'Categoría constructiva única en zona residencial de Adrogué. Techos altos, aberturas de madera maciza, parque arbolado, sótano habitable. 340m2.', 295000.00, 'USD', -34.795000, -58.384000, 'Drumond 600', 'AVAILABLE'),
('PROP-017', 'owner-999', 'Casa 4 Ambientes con Local Comercial', 'Propiedad en esquina estratégica de Ramos Mejía, ideal inversores. Casa familiar de 3 dormitorios en planta alta y local comercial abajo. 180m2.', 160000.00, 'USD', -34.647000, -58.557000, 'Av. San Martín 2100', 'AVAILABLE'),
('PROP-018', 'owner-999', 'Casa a Actualizar con Gran Lote', 'Estructura sólida a refaccionar en Ramos Mejía. Lote de 10x40 metros, fondo libre con árboles frutales, 2 dormitorios y garage. 90m2 cubiertos.', 85000.00, 'USD', -34.649000, -58.569000, 'Pringles 1800', 'AVAILABLE'),
('PROP-019', 'owner-999', 'Chalet Clásico de Tejas Coloniales', 'Living comedor en desnivel en Adrogué. Altillo/playroom muy espacioso, 3 dormitorios, garage cubierto para dos autos y fondo con parrilla. 200m2.', 210000.00, 'USD', -34.801000, -58.395000, 'Amenedo 1100', 'AVAILABLE'),
('PROP-020', 'owner-999', 'Casa Sustentable de Estreno', 'Construcción modular en seco de alta eficiencia en Francisco Álvarez. Climatización solar instalada, 3 dormitorios, 2 baños y diseño bioclimático. 165m2.', 245000.00, 'USD', -34.629000, -58.868000, 'Gorriti 450', 'AVAILABLE');
