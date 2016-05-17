CREATE DATABASE `nparam_test` /*!40100 DEFAULT CHARACTER SET ascii */;

CREATE TABLE `tbl` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `id_string` varchar(255) NOT NULL,
  `type` int(11) NOT NULL DEFAULT '0',
  `int_value` int(11) NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_string_UNIQUE` (`id_string`),
  KEY `type_IDX` (`type`)
) ENGINE=InnoDB AUTO_INCREMENT=1001 DEFAULT CHARSET=ascii;





GRANT SELECT, INSERT ON `nparam_test`.* TO 'nparam_test'@'10.44.0.0/255.255.0.0';